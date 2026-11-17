package ui

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

type Docker struct {
	Socket string
	client *http.Client
}

func NewDocker(socket string) *Docker {
	if socket == "" {
		socket = "/var/run/docker.sock"
	}
	return &Docker{
		Socket: socket,
		client: &http.Client{
			Timeout: 30 * time.Second,
			Transport: &http.Transport{
				DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
					var d net.Dialer
					return d.DialContext(ctx, "unix", socket)
				},
			},
		},
	}
}

func (d *Docker) Stop(name string) error {
	return d.call(http.MethodPost, "/containers/"+name+"/stop?t=1")
}

func (d *Docker) Start(name string) error {
	return d.call(http.MethodPost, "/containers/"+name+"/start")
}

func (d *Docker) Kill(name string) error {
	return d.call(http.MethodPost, "/containers/"+name+"/kill?signal=SIGKILL")
}

func (d *Docker) Running(name string) (bool, error) {
	resp, err := d.client.Get("http://docker/containers/" + name + "/json")
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return false, fmt.Errorf("inspect %s: %s %s", name, resp.Status, strings.TrimSpace(string(raw)))
	}
	var body struct {
		State struct {
			Running bool `json:"Running"`
		} `json:"State"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		return false, err
	}
	return body.State.Running, nil
}

func (d *Docker) call(method, path string) error {
	req, err := http.NewRequest(method, "http://docker"+path, nil)
	if err != nil {
		return err
	}
	resp, err := d.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNoContent || resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusNotModified {
		return nil
	}
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	return fmt.Errorf("%s %s: %s %s", method, path, resp.Status, strings.TrimSpace(string(raw)))
}
