package main

import (
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sethvargo/go-githubactions"
)

func buildPackage(spec *RPMSpec) error {
	githubactions.Group(fmt.Sprintf("Building package \"%s\"", spec.Name))
	defer githubactions.EndGroup()

	// nolint:gosec // Subprocess launched with a potential tainted input or cmd arguments
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

	if _, err := fmt.Fprintf(repoConfig, "[local]\nname=Local development packages\nbaseurl=file://%s\nenabled=1\ngpgcheck=0\nprotect=1\npriority=1\n", repoOutput); err != nil {
		return nil, err
	}

	return func() error {
		githubactions.Group("Updating local repo")
		defer githubactions.EndGroup()

		return createRepo(repoCache, repoOutput, cwd)
	}, nil
}

func createRepo(cachePath, outputPath string, repoPath string) error {
	cmdArgs := []string{"-q", "-u"}

	if cachePath != "" {
		cmdArgs = append(cmdArgs, "-c", cachePath)
	}

	if outputPath != "" {
		cmdArgs = append(cmdArgs, "-o", outputPath)
	}

	cmdArgs = append(cmdArgs, repoPath)

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
	// nolint:gofumpt,gomnd
	if err := os.MkdirAll(destPath, 0755); err != nil {
		githubactions.Errorf(err.Error())

		return err
	}

	// spectool is broken for some spec files, use parsed spec as a workaround
	tempFile, err := ioutil.TempFile("", "spec-")
	if err != nil {
		return err
	}
	//goland:noinspection GoUnhandledErrorResult
	defer tempFile.Close()

	if _, err := spec.ParsedSpec.Seek(0, io.SeekStart); err != nil {
		return err
	}

	if _, err := io.Copy(tempFile, spec.ParsedSpec); err != nil {
		return err
	}

	if err := tempFile.Close(); err != nil {
		return err
	}

	cmd := exec.Command("spectool", "-g", "-C", destPath, tempFile.Name())
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
	// nolint:prealloc
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

			for _, devDep := range []string{
				depName,
				strings.TrimSuffix(depName, "-devel"),
				strings.TrimPrefix(depName, "lib"),
				strings.TrimSuffix(strings.TrimPrefix(depName, "lib"), "-devel"),
			} {
				if _, ok := specDefs[devDep]; ok {
					depPkgList = append(depPkgList, devDep)
				}
			}
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

func getRPMSpecs(specPaths []string) (map[string]*RPMSpec, error) {
	githubactions.Group("Reading RPM spec files")
	defer githubactions.EndGroup()

	var err error

	result := map[string]*RPMSpec{}

	for _, specPath := range specPaths {
		var spec *RPMSpec

		spec, err = NewRPMSpec(specPath)
		if err != nil {
			githubactions.Errorf("Skipping \"%s\": %s", specPath, err)

			continue
		}

		if err := spec.TestArch(); err != nil {
			githubactions.Errorf("Skipping \"%s\": %s", specPath, err)

			continue
		}

		result[spec.Name] = spec
	}

	return result, err
}

func installBuildDeps(spec *RPMSpec) error {
	githubactions.Group(fmt.Sprintf("Installing build dependencies for package \"%s\"", spec.Name))
	defer githubactions.EndGroup()

	// nolint:gosec // Subprocess launched with a potential tainted input or cmd arguments
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

// nolint:cyclop
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

	pkgList := make([]string, 0, len(rpmSpecs))

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

	buildSuccessful := make([]string, 0, len(jobQueue))

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
