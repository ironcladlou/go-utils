// This program will copy a binary from the host to a specified file inside an
// existing tagged image, preserving all the image's pre-existing metadata.
package main

import (
	"flag"
	"fmt"
	"os"
	"path"
	"strings"

	docker "github.com/fsouza/go-dockerclient"
)

func main() {
	var image, src, dest, cp string
	var useZ bool
	flag.StringVar(&image, "image", "", "The docker image to copy into")
	flag.StringVar(&src, "src", "", "The source file to copy")
	flag.StringVar(&dest, "dest", "", "The destination of the file in the container")
	flag.BoolVar(&useZ, "z", false, "Use the :z mount option")
	flag.StringVar(&cp, "cp", "/bin/cp", "The path to the cp binary in the image")
	flag.Parse()

	if len(image) == 0 {
		fmt.Println("image is required")
		os.Exit(1)
	}
	if len(src) == 0 {
		fmt.Println("src is required")
		os.Exit(1)
	}
	if len(dest) == 0 {
		fmt.Println("dest is required")
		os.Exit(1)
	}
	tokens := strings.Split(image, ":")
	if len(tokens) != 2 {
		fmt.Println("image must have image:tag format")
		os.Exit(1)
	}
	repo, tag := tokens[0], tokens[1]

	endpoint := "unix:///var/run/docker.sock"
	client, err := docker.NewClient(endpoint)
	if err != nil {
		fmt.Printf("Error connecting to Docker: %s\n", err)
		os.Exit(1)
	}

	fmt.Printf("Copying %s -> %s:%s\n", src, image, dest)
	origImage, err := client.InspectImage(image)
	if err != nil {
		fmt.Printf("Error inspecting image %q: %s\n", image, err)
		os.Exit(1)
	}

	tempDest := path.Join("/var/copy-to-image", dest)
	bind := fmt.Sprintf("%s:%s", src, tempDest)
	if useZ {
		bind = fmt.Sprintf("%s:%s:z", src, tempDest)
	}
	container, err := client.CreateContainer(docker.CreateContainerOptions{
		Config: &docker.Config{
			User:       "root",
			Image:      image,
			Entrypoint: []string{cp, "-v", tempDest, dest},
		},
		HostConfig: &docker.HostConfig{
			Binds: []string{bind},
		},
	})
	if err != nil {
		fmt.Printf("Error creating container: %s\n", err)
		os.Exit(2)
	}
	err = client.StartContainer(container.ID, nil)
	if err != nil {
		fmt.Printf("Error starting container: %s\n", err)
		os.Exit(1)
	}
	rc, err := client.WaitContainer(container.ID)
	if err != nil {
		fmt.Printf("Container wait failed: %s", err)
		os.Exit(1)
	}
	if rc != 0 {
		fmt.Printf("Container exited %d", rc)
		os.Exit(1)
	}
	fmt.Printf("Created container %s\n", container.ID)
	// If the slice is uninitialized, the client will send "null" for the
	// entrypoint, resulting in the containerConfig entrypoint being inherited.
	if len(origImage.Config.Entrypoint) == 0 {
		origImage.Config.Entrypoint = []string{}
	}
	newImage, err := client.CommitContainer(docker.CommitContainerOptions{
		Container:  container.ID,
		Repository: repo,
		Tag:        tag,
		Run:        origImage.Config,
	})
	if err != nil {
		fmt.Printf("Error committing container: %s\n", err)
		os.Exit(1)
	}
	fmt.Printf("Committed %s (image %s)\n", image, newImage.ID)
	err = client.RemoveContainer(docker.RemoveContainerOptions{ID: container.ID})
	if err != nil {
		fmt.Printf("Error removing container: %s\n", err)
	}
}
