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

func getJobQueue(pkgList []string, specDefs map[string]*RPMSpec) []*RPMSpec {
	return getJobQueueRecurse(pkgList, specDefs, map[string]struct{}{})
}

func getJobQueueRecurse(pkgList []string, specDefs map[string]*RPMSpec, processed map[string]struct{}) []*RPMSpec {
	var result []*RPMSpec

	for _, pkgName := range pkgList {
		if _, ok := processed[pkgName]; ok {
			githubactions.Debugf("Skipping package \"%s\", already processed", pkgName)
			continue
		}

		spec, ok := specDefs[pkgName]
		if !ok {
			continue
		}

		githubactions.Debugf("Package \"%s\" build dependencies: %s", pkgName, strings.Join(spec.BuildDeps, ", "))
		var depPkgList []string
		for _, depName := range spec.BuildDeps {
			if _, ok := processed[depName]; ok {
				githubactions.Debugf("Skipping build dependency \"%s\", already processed", depName)
				continue
			}

			if _, ok := specDefs[depName]; !ok {
				continue
			}

			depPkgList = append(depPkgList, depName)
		}

		for _, depSpec := range getJobQueueRecurse(depPkgList, specDefs, processed) {
			githubactions.Debugf("Build dependency \"%s\" will be built before \"%s\"", depSpec.Name, pkgName)
			processed[depSpec.Name] = struct{}{}
			result = append(result, depSpec)
		}

		githubactions.Debugf("Adding package \"%s\" to the queue", pkgName)
		processed[pkgName] = struct{}{}
		result = append(result, spec)
	}

	return result
}

func getRPMSpecs(p... string) (map[string]*RPMSpec, error) {
	githubactions.Group("Reading RPM spec files")
	defer githubactions.EndGroup()

	var err error
	result := map[string]*RPMSpec{}

	for _, p := range p {
		var spec *RPMSpec
		spec, err = NewRPMSpec(p)
		if err != nil {
			githubactions.Errorf("Skipping spec \"%s\": %s", p, err)
			continue
		}

		result[spec.Name] = spec
	}

	return result, err
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

	rpmSpecs, err := getRPMSpecs(os.Args[1:]...)
	if err != nil {
		exitCode = 1
	}

	githubactions.Group("Building jobs queue")
	var pkgList []string
	for p := range rpmSpecs {
		pkgList = append(pkgList, p)
	}

	jobQueue := getJobQueue(pkgList, rpmSpecs)

	githubactions.Debugf("Package build order: %s", jobQueue)
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
