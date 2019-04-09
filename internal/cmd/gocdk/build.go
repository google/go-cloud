package main

import (
	"bytes"
	"context"
	"flag"
	"io/ioutil"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/xerrors"
)

func build(ctx context.Context, pctx *processContext, args []string) error {
	f := flag.NewFlagSet("build", flag.ContinueOnError)
	list := f.Bool("list", false, "display Docker images of this project")
	// TODO(light): Figure out how to correctly exit with status code.
	if err := f.Parse(args); err != nil {
		return err
	}
	if *list {
		return listBuilds(ctx, pctx, f.Args())
	}
	return xerrors.New("not implemented")
}

func listBuilds(ctx context.Context, pctx *processContext, args []string) error {
	if len(args) != 0 {
		return usagef("usage: gocdk build --list")
	}
	moduleRoot, err := findModuleRoot(ctx, pctx.workdir)
	if err != nil {
		return xerrors.Errorf("find module root: %w", err)
	}
	imageName, err := moduleDockerImageName(moduleRoot)
	if err != nil {
		return xerrors.Errorf("find module root: %w", err)
	}
	c := exec.CommandContext(ctx, "docker", "images", imageName)
	c.Stdout = pctx.stdout
	c.Stderr = pctx.stderr
	if err := c.Run(); err != nil {
		return xerrors.Errorf("find module root: %w", err)
	}
	return nil
}

func moduleDockerImageName(moduleRoot string) (string, error) {
	dockerfilePath := filepath.Join(moduleRoot, "Dockerfile")
	dockerfile, err := ioutil.ReadFile(dockerfilePath)
	if err != nil {
		return "", xerrors.Errorf("finding module Docker image name in %s: %w", dockerfilePath, err)
	}
	imageName, err := parseImageNameFromDockerfile(dockerfile)
	if err != nil {
		return "", xerrors.Errorf("finding module Docker image name in %s: %w", dockerfilePath, err)
	}
	return imageName, nil
}

// parseImageNameFromDockerfile finds the magic "# gocdk-image:" comment in a
// Dockerfile and returns the image name.
func parseImageNameFromDockerfile(dockerfile []byte) (string, error) {
	const magic = "# gocdk-image:"
	commentStart := bytes.Index(dockerfile, []byte(magic))
	if commentStart == -1 {
		return "", xerrors.New("source does not contain the comment \"# gocdk-image:\"")
	}
	// TODO(light): Keep searching if comment does not start at beginning of line.
	nameStart := commentStart + len(magic)
	lenName := bytes.Index(dockerfile[nameStart:], []byte("\n"))
	if lenName == -1 {
		// No newline, go to end of file.
		lenName = len(dockerfile) - nameStart
	}
	name := string(dockerfile[nameStart : nameStart+lenName])
	return strings.TrimSpace(name), nil
}
