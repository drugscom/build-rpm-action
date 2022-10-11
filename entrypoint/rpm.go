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
	"runtime"
	"strings"
)

func getBuildDeps(spec io.ReadSeeker) ([]string, error) {
	params, err := getSpecParamArray(spec, "BuildRequires")
	if err != nil {
		return nil, err
	}

	result := make([]string, 0, len(params))

	processed := map[string]struct{}{}

	for _, s := range params {
		pkgName := regexp.MustCompile(`^[a-zA-Z][-._+a-zA-Z0-9]+`).FindString(s)

		if _, ok := processed[pkgName]; ok {
			// Ignore duplicated dependency
			continue
		}

		processed[pkgName] = struct{}{}

		result = append(result, pkgName)
	}

	return result, nil
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

func getSpecParam(spec io.ReadSeeker, key string) ([]string, error) {
	var result []string

	if _, err := spec.Seek(0, io.SeekStart); err != nil {
		return result, err
	}

	scanner := bufio.NewScanner(spec)
	for scanner.Scan() {
		line := scanner.Text()

		//nolint:gomnd
		matched := regexp.MustCompile(fmt.Sprintf(`^\s*%s:\s*`, key)).Split(line, 2)
		if len(matched) > 1 {
			result = append(result, matched[1])
		}
	}

	if err := scanner.Err(); err != nil {
		return result, err
	}

	return result, nil
}

func getSpecParamArray(spec io.ReadSeeker, key string) ([]string, error) {
	var result []string

	processed := map[string]struct{}{}

	params, err := getSpecParam(spec, key)
	if err != nil {
		return result, err
	}

	for _, s := range params {
		for _, param := range regexp.MustCompile(`[ ,]`).Split(s, -1) {
			param = strings.TrimSpace(param)

			if param == "" {
				continue
			}

			if _, ok := processed[param]; ok {
				// Ignore duplicated entries
				continue
			}

			processed[param] = struct{}{}

			result = append(result, param)
		}
	}

	return result, nil
}

func getSpecParamString(spec io.ReadSeeker, key string) (string, error) {
	params, err := getSpecParam(spec, key)
	if err != nil {
		return "", err
	}

	if len(params) != 1 {
		return "", fmt.Errorf("failed to determine a value for \"%s\"", key)
	}

	return params[0], nil
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
	Name          string
	Path          string
	BuildDeps     []string
	BuildPath     string
	ExcludeArch   []string
	ExclusiveArch []string
	ParsedSpec    *bytes.Reader
}

func (s *RPMSpec) String() string {
	return s.Name
}

func (s *RPMSpec) TestArch() error {
	var buildArch string

	switch goArch := runtime.GOARCH; goArch {
	case "amd64":
		buildArch = "x86_64"
	case "arm64":
		buildArch = "aarch64"
	default:
		buildArch = goArch
	}

	for _, arch := range s.ExcludeArch {
		if buildArch == arch {
			return fmt.Errorf("architecture \"%s\" found in spec exclude list (%s)", buildArch, strings.Join(s.ExcludeArch, ", "))
		}
	}

	if len(s.ExclusiveArch) == 0 {
		return nil
	}

	for _, arch := range s.ExclusiveArch {
		if buildArch == arch {
			return nil
		}
	}

	return fmt.Errorf("architecture \"%s\" not found in spec exclusive list (%s)", buildArch, strings.Join(s.ExclusiveArch, ", "))
}

func NewRPMSpec(specPath string) (*RPMSpec, error) {
	// Ensure spec file is accessible
	if _, err := os.Stat(specPath); err != nil {
		return nil, err
	}

	buildPath, err := getBuildPath(specPath)
	if err != nil {
		return nil, err
	}

	parsedSpec, err := parseSpec(specPath)
	if err != nil {
		return nil, err
	}

	packageName, err := getSpecParamString(parsedSpec, "Name")
	if err != nil {
		return nil, err
	}

	if packageName == "" {
		return nil, fmt.Errorf("could not determine package name")
	}

	buildDeps, err := getBuildDeps(parsedSpec)
	if err != nil {
		return nil, err
	}

	excludeArchs, err := getSpecParamArray(parsedSpec, "ExcludeArch")
	if err != nil {
		return nil, err
	}

	exclusiveArchs, err := getSpecParamArray(parsedSpec, "ExclusiveArch")
	if err != nil {
		return nil, err
	}

	return &RPMSpec{
		Name:          packageName,
		Path:          specPath,
		BuildDeps:     buildDeps,
		BuildPath:     buildPath,
		ExcludeArch:   excludeArchs,
		ExclusiveArch: exclusiveArchs,
		ParsedSpec:    parsedSpec,
	}, nil
}
