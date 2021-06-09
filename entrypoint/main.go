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

	cmd := exec.Command("rpmbuild", "-ba", "--nocheck", spec.Path,
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

func cleanMetadata() error {
	cmd := exec.Command("yum", "clean", "metadata")
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	githubactions.Debugf(cmd.String())
	if err := cmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func createLocalRepo() (func() error, error) {
	githubactions.Group("Creating local repo")
	defer githubactions.EndGroup()

	cwd, err := os.Getwd()
	if err != nil {
		return nil, err
	}

	repoOutput, err := os.MkdirTemp("", "yumrepo_*")
	if err != nil {
		return nil, err
	}

	repoCache, err := os.MkdirTemp("", "createrepo-cache_*")
	if err != nil {
		return nil, err
	}

	if err := createRepo(repoCache, repoOutput, cwd); err != nil {
		return nil, err
	}

	if err := cleanMetadata(); err != nil {
		return nil, err
	}

	repoConfig, err := os.Create("/etc/yum.repos.d/local.repo")
	if err != nil {
		return nil, err
	}

	if _, err := fmt.Fprintf(repoConfig, "[local]\nname=Local development packages\nbaseurl=file://%s\nenabled=1\ngpgcheck=0\nprotect=1", repoOutput); err != nil {
		return nil, err
	}

	return func() error {
		githubactions.Group("Updating local repo")
		defer githubactions.EndGroup()
		return createRepo(repoCache, repoOutput, cwd)
	}, nil
}

func createRepo(cachePath, outputPath string, p string) error {
	cmdArgs := []string{"-q", "-u", p}

	if cachePath != "" {
		cmdArgs = append(cmdArgs, "-c", cachePath)
	}
	if outputPath != "" {
		cmdArgs = append(cmdArgs, "-o", outputPath)
	}

	cmdArgs = append(cmdArgs, p)

	cmd := exec.Command("createrepo", cmdArgs...)
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
	if err := os.MkdirAll(destPath, 0755); err != nil {
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

func getRPMSpecs(p []string) (map[string]*RPMSpec, error) {
	githubactions.Group("Reading RPM spec files")
	defer githubactions.EndGroup()

	var err error
	result := map[string]*RPMSpec{}

	for _, p := range p {
		var spec *RPMSpec
		spec, err = NewRPMSpec(p)
		if err != nil {
			githubactions.Errorf("Skipping \"%s\": %s", p, err)
			continue
		}
		if err := spec.TestArch(); err != nil {
			githubactions.Errorf("Skipping \"%s\": %s", p, err)
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

func installExtraPackages(packages ...string) error {
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
	yumExtras := GetInputAsArray("yum-extras")
	if len(yumExtras) > 0 {
		if err := installExtraPackages(yumExtras...); err != nil {
			githubactions.Fatalf(err.Error())
		}
	}

	rpmSpecs, err := getRPMSpecs(GetArgsAsArray(os.Args[1:]))
	if err != nil {
		githubactions.Fatalf(err.Error())
	}

	githubactions.Group("Building jobs queue")
	var pkgList []string
	for p := range rpmSpecs {
		pkgList = append(pkgList, p)
	}

	jobQueue := getJobQueue(pkgList, rpmSpecs)

	githubactions.Debugf("Package build order: %s", jobQueue)
	githubactions.EndGroup()

	updateLocalRepo, err := createLocalRepo()
	if err != nil {
		githubactions.Fatalf(err.Error())
	}

	var buildSuccessful []string
	for _, spec := range jobQueue {
		githubactions.Debugf("Building package \"%s\" using spec file \"%s\"", spec.Name, spec.Path)

		if err := installBuildDeps(spec); err != nil {
			githubactions.Fatalf(err.Error())
		}

		if err := downloadSources(spec); err != nil {
			githubactions.Fatalf(err.Error())
		}

		if err := buildPackage(spec); err != nil {
			githubactions.Fatalf(err.Error())
		}
		buildSuccessful = append(buildSuccessful, spec.Path)

		//goland:noinspection GoNilness
		if err := updateLocalRepo(); err != nil {
			githubactions.Fatalf(err.Error())
		}
		if err := cleanMetadata(); err != nil {
			githubactions.Fatalf(err.Error())
		}
	}

	githubactions.SetOutput("successful", strings.Join(buildSuccessful, ","))
}
