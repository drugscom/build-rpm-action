package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sethvargo/go-githubactions"
)


func buildPackage(spec *RPMSpec) error {
	githubactions.Group(fmt.Sprintf("Building package \"%s\"", spec.Name))
	defer githubactions.EndGroup()

	cmd := exec.Command("rpmbuild","-ba", "--nocheck", spec.Path,
		"--define", fmt.Sprintf("_topdir %s", spec.BuildPath),
		"--define", "_build_name_fmt %%{NAME}-%%{VERSION}-%%{RELEASE}.%%{ARCH}.rpm",
	)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func downloadSources(spec *RPMSpec) error {
	githubactions.Group(fmt.Sprintf("Downloading sources for package \"%s\"", spec.Name))
	defer githubactions.EndGroup()

	destPath := path.Join(spec.BuildPath, "SOURCES")
	if err := os.MkdirAll(destPath,0755); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}


	cmd := exec.Command("spectool", "-g", "-C", destPath, spec.Path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func getJobQueue(specDefs map[string]*RPMSpec) []*RPMSpec {
	var result []*RPMSpec
	processed := map[string]*RPMSpec{}

	for pkgName, spec := range specDefs {
		if _, ok := processed[pkgName]; ok {
			githubactions.Debugf("Skipping package \"%s\", already processed", pkgName)
			continue
		}

		githubactions.Debugf("Package \"%s\" has build dependencies: %s", pkgName, strings.Join(spec.BuildDeps, ", "))
		depSpecDefs := map[string]*RPMSpec{}
		for _, depName := range spec.BuildDeps {
			if depSpec, ok := specDefs[depName]; ok {
				githubactions.Debugf("Build dependency \"%s\" found in spec definitions", pkgName)
				depSpecDefs[depName] = depSpec
			}
		}

		if len(depSpecDefs) > 0 {
			githubactions.Debugf("Getting build dependency queue for package \"%s\"", pkgName)
			depSpecQueue := getJobQueue(depSpecDefs)

			for _, depSpec := range depSpecQueue {
				if _, ok := processed[depSpec.Name]; ok {
					githubactions.Debugf("Skipping build dependency \"%s\", already processed", pkgName)
					continue
				}

				githubactions.Debugf("Adding build dependency \"%s\" to the queue", pkgName)
				processed[depSpec.Name] = depSpec
				result = append(result, depSpec)
			}

		}

		githubactions.Debugf("Adding package \"%s\" to the queue", pkgName)
		result = append(result, spec)
	}

	return result
}

func installBuildDeps(spec *RPMSpec) error {
	githubactions.Group(fmt.Sprintf("Installing build dependencies for package \"%s\"", spec.Name))
	defer githubactions.EndGroup()

	cmd := exec.Command("yum-builddep", "-y", spec.Path)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func installExtraPackages(packages... string) error {
	githubactions.Group("Installing extra packages")
	defer githubactions.EndGroup()

	cmdArgs := append([]string{"-y", "install"}, packages...)

	cmd := exec.Command("yum", cmdArgs...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}


func main() {
	exitCode := 0

	yumExtras := GetInputAsArray("yum-extras")
	if len(yumExtras) > 0 {
		if err := installExtraPackages(yumExtras...); err != nil {
			githubactions.Fatalf(err.Error())
		}
	}

	rpmSpecs := map[string]*RPMSpec{}

	for _, p := range os.Args[1:] {
		spec, err := NewRPMSpec(p)
		if err != nil {
			githubactions.Errorf("Skipping spec \"%s\": %s", p, err)
			exitCode = 1
			continue
		}

		rpmSpecs[spec.Name] = spec
	}

	githubactions.Group("Building jobs queue")
	jobQueue := getJobQueue(rpmSpecs)
	githubactions.EndGroup()

	for _, spec := range jobQueue {
		githubactions.Debugf("Building package \"%s\" using spec file \"%s\"", spec.Name, spec.Path)

		if err := installBuildDeps(spec); err != nil {
			exitCode = 1
			continue
		}

		if err := downloadSources(spec); err != nil {
			exitCode = 1
			continue
		}

		if err := buildPackage(spec); err != nil {
			exitCode = 1
			continue
		}
	}

	os.Exit(exitCode)
}
