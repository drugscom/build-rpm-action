package main

import (
	"bufio"
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"regexp"
	"strings"
)

func getBuildDeps(spec io.ReadSeeker) ([]string, error) {
	if _, err := spec.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}

	var buildDeps []string
	buildDepsMap := map[string]struct{}{}

	scanner := bufio.NewScanner(spec)
	for scanner.Scan() {
		line := scanner.Text()

		matched := regexp.MustCompile(`^\s*BuildRequires:\s*`).Split(line, 2)
		if len(matched) < 2 {
			continue
		}
		line = matched[1]

		for _, s := range regexp.MustCompile(`[ ,]`).Split(line, -1) {
			pkgName := regexp.MustCompile(`^[a-zA-Z][-._+a-zA-Z0-9]+`).FindString(s)
			if pkgName == "" {
				continue
			}

			// Ignore -devel suffix (will likely be defined inside the main package spec)
			pkgName = strings.TrimSuffix(pkgName, "-devel")

			if _, ok := buildDepsMap[pkgName]; ok {
				// Ignore duplicated dependency
				continue
			}

			buildDepsMap[pkgName] = struct{}{}
			buildDeps = append(buildDeps, pkgName)
		}
	}
	if err := scanner.Err(); err != nil {
		return buildDeps, err
	}


	return buildDeps, nil
}

func getBuildPath(p string) (string, error) {
	// Assume spec is inside a SPECS directory in the build root
	result := path.Dir(path.Dir(p))

	// Make sure we return an absolute path
	if !path.IsAbs(result) {
		cwd, err := os.Getwd()
		if err != nil {
			return "", err
		}
		result = path.Join(cwd, result)
	}

	return result, nil
}

func getPackageName(spec io.ReadSeeker) (string, error) {
	if _, err := spec.Seek(0, io.SeekStart); err != nil {
		return "", err
	}

	scanner := bufio.NewScanner(spec)
	for scanner.Scan() {
		line := scanner.Text()

		matched := regexp.MustCompile(`^\s*Name:\s*`).Split(line, 2)
		if len(matched) > 1 {
			pkgName := matched[1]
			return pkgName, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}

	return "", nil
}

func parseSpec(p string) (*bytes.Reader, error) {
	cmdStdOut := &bytes.Buffer{}
	cmd := exec.Command("rpmspec", "-P", p)
	cmd.Stdout = cmdStdOut
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		return nil, err
	}

	return bytes.NewReader(cmdStdOut.Bytes()), nil
}


type RPMSpec struct {
	Name      string
	Path      string
	BuildDeps []string
	BuildPath string
}

func (s *RPMSpec) String() string {
	return s.Name
}

func NewRPMSpec(p string) (*RPMSpec, error) {
	// Ensure spec file is accessible
	if _, err := os.Stat(p); err != nil {
		return nil, err
	}

	buildPath, err := getBuildPath(p)
	if err != nil {
		return nil, err
	}

	parsedSpec, err := parseSpec(p)
	if err != nil {
		return nil, err
	}

	pkgName, err := getPackageName(parsedSpec)
	if err != nil {
		return nil, err
	}
	if pkgName == "" {
		return nil, fmt.Errorf("could not find package Name")
	}

	buildDeps, err := getBuildDeps(parsedSpec)
	if err != nil {
		return nil, err
	}


	return &RPMSpec{
		Name:      pkgName,
		Path:      p,
		BuildDeps: buildDeps,
		BuildPath: buildPath,
	}, nil
}
