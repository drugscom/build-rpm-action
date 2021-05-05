package main

import (
	"fmt"
	"os"
	"os/exec"
	"path"
	"strings"

	"github.com/sethvargo/go-githubactions"
)


func buildPackage(spec string) error {
	packageName := strings.TrimSuffix(path.Base(spec), path.Ext(spec))

	githubactions.Group(fmt.Sprintf("Building package %s", packageName))
	defer githubactions.EndGroup()

	if err := buildPackageTasks(spec); err != nil {
		githubactions.Errorf("Error building package %s: %s", packageName, err)
		return err
	}

	return nil
}

func buildPackageTasks(spec string) error {
	if _, err := os.Stat(spec); err != nil {
		return err
	}

	// Assume spec is inside a SPECS directory in the build root
	buildDir := path.Dir(path.Dir(spec))

	// rpmbuild expects an absolute path
	if !path.IsAbs(buildDir) {
		cwd, err := os.Getwd()
		if err != nil {
			return err
		}
		buildDir = path.Join(cwd, buildDir)
	}

	if err := installBuildDeps(spec); err != nil {
		return err
	}

	if err := downloadSources(spec, path.Join(buildDir, "SOURCES")); err != nil {
		return err
	}

	rpmbuildCmd := exec.Command("rpmbuild","-ba", "--nocheck", spec,
		"--define", fmt.Sprintf("_topdir %s", buildDir),
		"--define", "_build_name_fmt %%{NAME}-%%{VERSION}-%%{RELEASE}.%%{ARCH}.rpm",
	)
	rpmbuildCmd.Stdout = os.Stdout
	rpmbuildCmd.Stderr = os.Stderr
	if err := rpmbuildCmd.Run(); err != nil {
		return err
	}

	return nil
}


func downloadSources(spec, dest string) error {
	githubactions.Group("Downloading sources")
	defer githubactions.EndGroup()

	if err := os.MkdirAll(dest,0755); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	tempSpec, err := os.CreateTemp("", "*")
	if err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	rpmspecCmd := exec.Command("rpmspec", "-P", spec)
	rpmspecCmd.Stdout = tempSpec
	rpmspecCmd.Stderr = os.Stderr
	if err := rpmspecCmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	spectoolCmd := exec.Command("spectool", "-g", "-C", dest, tempSpec.Name())
	spectoolCmd.Stdout = os.Stdout
	spectoolCmd.Stderr = os.Stderr
	if err := spectoolCmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}

func installBuildDeps(spec string) error {
	githubactions.Group("Installing build dependencies")
	defer githubactions.EndGroup()

	builddepCmd := exec.Command("yum-builddep", "-y", spec)
	builddepCmd.Stdout = os.Stdout
	builddepCmd.Stderr = os.Stderr

	if err := builddepCmd.Run(); err != nil {
		githubactions.Errorf(err.Error())
		return err
	}

	return nil
}


func main() {
	exitCode := 0

	yumExtras := GetInputAsArray("yum-extras")
	if len(yumExtras) > 0 {
		githubactions.Group("Installing additional packages")
		defer githubactions.EndGroup()

		cmdArgs := append([]string{"-y", "install"}, yumExtras...)

		yumCmd := exec.Command("yum", cmdArgs...)
		yumCmd.Stdout = os.Stdout
		yumCmd.Stderr = os.Stderr

		if err := yumCmd.Run(); err != nil {
			githubactions.Fatalf("Failed to install additional packages. %s", err)
		}

	}

	for _, spec := range os.Args[1:] {
		if err := buildPackage(spec); err != nil {
			exitCode = 1
		}
	}

	os.Exit(exitCode)
}
