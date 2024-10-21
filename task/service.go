package task

import (
	"context"
	"io"
	"log"
	"math"
	"os"
	"time"

	// "github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/image"
	// "github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
    "github.com/docker/docker/pkg/stdcopy"
	"github.com/docker/go-connections/nat"
	"github.com/google/uuid"
	// "github.com/opencontainers/runtime-spec/specs-go"
)

type State int

const (
	Pending State = iota
	Scheduled
	Running
	Completed
	Failed
)

type Task struct {
	ID            uuid.UUID
	Name          string
	State         State
	Image         string
	CPU           float64
	Memory        int
	Disk          int
	ExposedPorts  nat.PortSet
	PortBindings  map[string]string
	RestartPolicy string
	StartAt       time.Time
	FinishTime    time.Time
}

type TaskEvent struct {
	ID        uuid.UUID
	State     State
	Timestamp time.Time
	Task      Task
}

type Config struct {
	Name          string
	AttachStdin   bool
	AttachStdout  bool
	AttachStderr  bool
	ExposedPorts  nat.PortSet
	Cmd           []string
	Image         string
	Cpu           float64
	Memory        int64
	Disk          int
	Env           []string
	RestartPolicy container.RestartPolicyMode
}

type Docker struct {
	Client *client.Client
	Config *Config
}

type DockerResult struct {
	Error       error
	Action      string
	ContainerID string
	Result      string
}

func (d *Docker) Run() DockerResult {
	ctx := context.Background()
	reader, err := d.Client.ImagePull(ctx, d.Config.Image, image.PullOptions{})

	if err != nil {
		log.Printf("Error pulling image %s: %v\n ", d.Config.Image, err)
		return DockerResult{Error: err, Action: "pull"}
	}

	io.Copy(os.Stdout, reader)

	rp := container.RestartPolicy{
		Name: d.Config.RestartPolicy,
	}

	r := container.Resources{
		Memory:   d.Config.Memory,
		NanoCPUs: int64(d.Config.Cpu * math.Pow(10, 9)),
	}

	cc := container.Config{
		Image:        d.Config.Image,
		Tty:          false,
		Env:          d.Config.Env,
		ExposedPorts: d.Config.ExposedPorts,
	}

	hc := container.HostConfig{
		RestartPolicy:   rp,
		Resources:       r,
		PublishAllPorts: true,
	}

    resp, err := d.Client.ContainerCreate(ctx, &cc, &hc, nil, nil, d.Config.Name)

	if err != nil {
		log.Printf("Error creating container %s: %v\n", d.Config.Name, err)
		return DockerResult{Error: err, Action: "create"}
	}

	err = d.Client.ContainerStart(ctx, resp.ID, container.StartOptions{})

	if err != nil {
		log.Printf("Error starting container %s: %v\n", d.Config.Name, err)
		return DockerResult{Error: err, Action: "start"}
	}

	//d.Config.Runtime.ContainerID = resp.ID

	out, err := d.Client.ContainerLogs(
		ctx,
		resp.ID,
		container.LogsOptions{ShowStdout: true, ShowStderr: true},
	)

	if err != nil {
		log.Printf("Error getting logs for container %s: %v\n", resp.ID, err)
		return DockerResult{Error: err, Action: "logs"}
	}

	stdcopy.StdCopy(os.Stdout, os.Stderr, out)
	return DockerResult{ContainerID: resp.ID, Action: "start", Result: "success"}
}

func (d *Docker) Stop(id string) DockerResult {
	log.Printf("Stopping container %v\n", id)

	ctx := context.Background()
    err := d.Client.ContainerStop(ctx, id, container.StopOptions{})

	if err != nil {
		log.Printf("Error stopping container %s: %v\n", id, err)
		return DockerResult{Error: err, Action: "stop"}
	}

	err = d.Client.ContainerRemove(ctx, id, container.RemoveOptions{
		RemoveVolumes: true,
		RemoveLinks:   false,
		Force:         false,
	})

	if err != nil {
		log.Printf("Error removing container %s: %v\n", id, err)
		return DockerResult{Error: err, Action: "remove"}
	}

	return DockerResult{ContainerID: id, Action: "stop", Result: "success", Error: nil}
}
