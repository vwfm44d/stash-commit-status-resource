package resource

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"time"
)

type Source struct {
	Host                string `json:"host"`
	Username            string `json:"username"`
	Password            string `json:"password"`
	RetryAttempts       int    `json:"retry_attempts"`
	SkipSSLVerification bool   `json:"skip_ssl_verification"`
}

type Version struct {
	Ref string `json:"ref"`
}

type Params struct {
	Repository  string `json:"repository"`
	Commit      string `json:"commit"`
	State       string `json:"state"`
	Description string `json:"description"`
}

type Metadata []MetadataItem

type MetadataItem struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

type Response struct {
	Version  Version  `json:"version"`
	Metadata Metadata `json:"metadata"`
}

type Request struct {
	Source  Source  `json:"source"`
	Version Version `json:"version"`
	Params  Params  `json:"params"`
}

func Log(format string, values ...interface{}) {
	fmt.Fprintf(os.Stderr, format, values...)
}

func Error(format string, values ...interface{}) {
	fmt.Fprintf(os.Stderr, format, values...)
	os.Exit(1)
}

func Output(v interface{}) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}

	fmt.Fprintf(os.Stdout, "%s", b)
	return nil
}

func Put(req Request) error {
	src := req.Source
	client := NewStashClient(src.Host, src.Username, src.Password, src.SkipSSLVerification)

	cmd := exec.Command("git", "rev-parse", "--short=40", "HEAD")
	cmd.Dir = fmt.Sprintf("%s/%s", os.Args[1], req.Params.Repository)
	commit, err := cmd.CombinedOutput()
	if err != nil {
		return err
	}

	commit = bytes.TrimSuffix(commit, []byte("\n"))
	Log("Setting build status for %s\n", commit)

	status := Status{
		State:       req.Params.State,
		Key:         os.Getenv("BUILD_JOB_NAME"),
		Name:        fmt.Sprintf("%s-%s", os.Getenv("BUILD_JOB_NAME"), os.Getenv("BUILD_ID")),
		Description: req.Params.Description,
		URL:         getBuildURL(),
	}

	Log("Build status %#v\n", status)
	err = client.SetBuildStatus(string(commit), status)
	attempts := req.Source.RetryAttempts
	for err != nil && attempts > 0 {
		Log("Failed to set build status, retrying...\n")
		err = client.SetBuildStatus(string(commit), status)
		attempts--
		time.Sleep(time.Second)
	}

	if err != nil {
		Error("Failed to set the build status after %d attempts %s", req.Source.RetryAttempts, err)
	}
	Log("Status set successfully\n")

	version := Version{Ref: string(commit)}
	result := Response{
		Version: version,
		Metadata: Metadata{
			{Name: "commit", Value: version.Ref},
			{Name: "date_added", Value: strconv.FormatInt(status.DateAdded, 10)},
			{Name: "description", Value: status.Description},
			{Name: "key", Value: status.Key},
			{Name: "name", Value: status.Name},
			{Name: "state", Value: status.State},
			{Name: "url", Value: status.URL},
		},
	}

	return Output(result)
}

func getBuildURL() string {
	atc := os.Getenv("ATC_EXTERNAL_URL")
	team := "main" // hard-coded until https://github.com/concourse/concourse/issues/616 is resolved
	if t := os.Getenv("BUILD_TEAM_NAME"); t != "" {
		team = t
	}
	pipeline := os.Getenv("BUILD_PIPELINE_NAME")
	job := os.Getenv("BUILD_JOB_NAME")
	build := os.Getenv("BUILD_NAME")
	return fmt.Sprintf(`%s/teams/%s/pipelines/%s/jobs/%s/builds/%s`, atc, team, pipeline, job, build)
}
