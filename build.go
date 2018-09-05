// +build ignore

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	runner "github.com/SentientTechnologies/studio-go-runner/internal/runner"

	"github.com/karlmutch/duat"
	"github.com/karlmutch/duat/version"
	logxi "github.com/karlmutch/logxi/v1"

	"github.com/karlmutch/errors" // Forked copy of https://github.com/jjeffery/errors
	"github.com/karlmutch/stack"  // Forked copy of https://github.com/go-stack/stack

	"github.com/karlmutch/envflag" // Forked copy of https://github.com/GoBike/envflag
)

var (
	logger = logxi.New("build.go")

	verbose     = flag.Bool("v", false, "When enabled will print internal logging for this tool")
	recursive   = flag.Bool("r", false, "When enabled this tool will visit any sub directories that contain main functions and build in each")
	userDirs    = flag.String("dirs", ".", "A comma separated list of root directories that will be used a starting points looking for Go code, this will default to the current working directory")
	imageOnly   = flag.Bool("image-only", false, "Used to start at the docker build step, will progress to github release, if not set the build halts after compilation")
	githubToken = flag.String("github-token", "", "If set this will automatically trigger a release of the binary artifacts to github at the current version")
)

func usage() {
	fmt.Fprintln(os.Stderr, path.Base(os.Args[0]))
	fmt.Fprintln(os.Stderr, "usage: ", os.Args[0], "[options]       build tool (build.go)      ", version.GitHash, "    ", version.BuildTime)
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Arguments")
	fmt.Fprintln(os.Stderr, "")
	flag.PrintDefaults()
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "Environment Variables:")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "options can also be extracted from environment variables by changing dashes '-' to underscores and using upper case.")
	fmt.Fprintln(os.Stderr, "")
	fmt.Fprintln(os.Stderr, "log levels are handled by the LOGXI env variables, these are documented at https://github.com/mgutz/logxi")
}

func init() {
	flag.Usage = usage
}

func main() {
	// This code is run in the same fashion as a script and should be co-located
	// with the component that is being built

	// Parse the CLI flags
	if !flag.Parsed() {
		envflag.Parse()
	}

	if *verbose {
		logger.SetLevel(logxi.LevelDebug)
	}

	// First assume that the directory supplied is a code directory
	rootDirs := strings.Split(*userDirs, ",")
	dirs := []string{}

	err := errors.New("")

	// If this is a recursive build scan all inner directories looking for go code
	// and build it if there is code found
	//
	if *recursive {
		for _, dir := range rootDirs {
			// Dont allow the vednor directory to creep in
			if filepath.Base(dir) == "vendor" {
				continue
			}

			// Otherwise look for meanful code that can be run either as tests
			// or as a standard executable
			for _, funct := range []string{"main", "TestMain"} {
				// Will auto skip any vendor directories found
				found, err := duat.FindGoDirs(dir, funct)
				if err != nil {
					fmt.Fprintln(os.Stderr, err.Error())
					os.Exit(-1)
				}
				dirs = append(dirs, found...)
			}
		}
	} else {
		dirs = rootDirs
	}

	// Now remove duplicates within the list of directories that we can potentially
	// visit during builds, removing empty strings
	{
		lastSeen := ""
		deDup := make([]string, 0, len(dirs))
		sort.Strings(dirs)
		for _, dir := range dirs {
			if dir != lastSeen {
				deDup = append(deDup, dir)
				lastSeen = dir
			}
		}
		dirs = deDup
	}

	logger.Debug(fmt.Sprintf("dirs %v", dirs))

	// Take the discovered directories and build them from a deduped
	// directory set
	for _, dir := range dirs {
		if _, err = runBuild(dir, "README.md"); err != nil {
			logger.Warn(err.Error())
			break
		}
	}

	outputs := []string{}
	if err == nil {
		for _, dir := range dirs {
			localOut, err := runRelease(dir, "README.md")
			outputs = append(outputs, localOut...)
			if err != nil {
				break
			}
		}
	}
	logger.Debug(fmt.Sprintf("built %s %v", strings.Join(outputs, ", "), stack.Trace().TrimRuntime()))

	for _, output := range outputs {
		fmt.Fprintln(os.Stdout, output)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(-2)
	}
}

// runBuild is used to restore the current working directory after the build itself
// has switch directories
//
func runBuild(dir string, verFn string) (outputs []string, err errors.Error) {

	logger.Info(fmt.Sprintf("visiting %s", dir))

	// Switch to the targets directory while the build is being done.  The defer will
	// return us back to ground 0
	cwd, errGo := os.Getwd()
	if errGo != nil {
		return outputs, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer func() {
		if errGo = os.Chdir(cwd); errGo != nil {
			logger.Warn("The original directory could not be restored after the build completed")
			if err == nil {
				err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	// Gather information about the current environment. also changes directory to the working area
	md, err := duat.NewMetaData(dir, verFn)
	if err != nil {
		return outputs, err
	}

	// Are we running inside a container runtime such as docker
	runtime, err := md.ContainerRuntime()
	if err != nil {
		return nil, err
	}

	// If we are in a container then do a stock compile, if not then it is
	// time to dockerize all the things
	if len(runtime) != 0 {
		logger.Info(fmt.Sprintf("building %s", dir))
		outputs, err = build(md)
	}

	if err == nil && !*imageOnly {
		logger.Info(fmt.Sprintf("testing %s", dir))
		out, errs := test(md)
		if len(errs) != 0 {
			return nil, errs[0]
		}
		outputs = append(outputs, out...)
	}

	if len(runtime) == 0 {
		// Dont Dockerize in the main root directory of a project.  The root
		// dir Dockerfile is for a projects build container typically.
		if dir != "." {
			logger.Info(fmt.Sprintf("dockerizing %s", dir))
			if err := dockerize(md); err != nil {
				return nil, err
			}
			// Check for a bin directory and continue if none
			if _, errGo := os.Stat("./bin"); errGo == nil {
				outputs, err = md.GoFetchBuilt()
			}
		}
	}

	if err != nil {
		return nil, err
	}

	return outputs, err
}

func runRelease(dir string, verFn string) (outputs []string, err errors.Error) {

	outputs = []string{}

	cwd, errGo := os.Getwd()
	if errGo != nil {
		return outputs, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	defer func() {
		if errGo = os.Chdir(cwd); errGo != nil {
			logger.Warn("The original directory could not be restored after the build completed")
			if err == nil {
				err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
		}
	}()

	// Gather information about the current environment. also changes directory to the working area
	md, err := duat.NewMetaData(dir, verFn)
	if err != nil {
		return outputs, err
	}

	// Are we running inside a container runtime such as docker
	runtime, err := md.ContainerRuntime()
	if err != nil {
		return outputs, err
	}

	if len(*githubToken) != 0 {
		if _, errGo := os.Stat("./bin"); errGo == nil {
			if outputs, err = md.GoFetchBuilt(); err != nil {
				return outputs, err
			}
		}

		logger.Info(fmt.Sprintf("github releasing %s", dir))
		err = md.CreateRelease(*githubToken, "", outputs)
	}

	if len(runtime) == 0 {
		return outputs, err
	}

	// Now work on the AWS push

	// Now do the Azure push

	return outputs, err
}

// build performs the default build for the component within the directory specified, but does
// no further than producing binaries that need to be done within a isolated container
//
func build(md *duat.MetaData) (outputs []string, err errors.Error) {

	// Before beginning purge the bin directory into which our files are saved
	// for downstream packaging etc
	os.RemoveAll("./bin")
	if errGo := os.MkdirAll("./bin", os.ModePerm); errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}

	opts := []string{
		"-a",
	}

	// Do the NO_CUDA executable first as we dont want to overwrite the
	// executable that uses the default output file name in the build
	targets, err := md.GoBuild([]string{"NO_CUDA"}, opts)
	if err != nil {
		return nil, err
	}

	// Copy the targets to the destination based on their types
	for _, target := range targets {
		base := "./bin/" + path.Base(target)
		dest := base + "-cpu"
		logger.Info(fmt.Sprintf("renaming %s to %s", base, dest))

		if errGo := os.Rename(base, dest); errGo != nil {
			return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("src", target).With("dest", dest)
		}
		outputs = append(outputs, dest)
	}
	if targets, err = md.GoBuild([]string{}, opts); err != nil {
		return nil, err
	}
	outputs = append(outputs, targets...)

	return outputs, nil
}

func CudaPresent() bool {
	libPaths := strings.Split(os.Getenv("LD_LIBRARY_PATH"), ":")
	for _, aPath := range libPaths {
		if _, errGo := os.Stat(filepath.Join(aPath, "libnvidia-ml.so.1")); errGo == nil {
			return true
		}
	}
	return false
}

func GPUPresent() bool {
	if _, errGo := os.Stat("/dev/nvidiactl"); errGo != nil {
		return false
	}
	if _, errGo := os.Stat("/dev/nvidia0"); errGo != nil {
		return false
	}
	// TODO We can check for a GPU by using nvidia-smi -L
	return true

}

// test inspects directories within the project that contain test cases, implemented
// using the standard go build _test.go file names, and runs those tests that
// the hardware provides support for
//
func test(md *duat.MetaData) (outputs []string, errs []errors.Error) {

	opts := []string{
		"-a",
		"-v",
	}

	if !GPUPresent() {
		opts = append(opts, "--no-gpu")
	}

	tags := []string{}

	// Look for CUDA Hardware and set the build flags for the tests based
	// on its presence
	if !CudaPresent() {
		tags = append(tags, "NO_CUDA")
	}

	// Go through the directories looking for test files
	testDirs := []string{}
	rootDirs := []string{"."}

	// If this is a recursive build scan all inner directories looking for go code
	// and save these somewhere for us to comeback and look for test code
	//
	if *recursive {
		dirs := []string{}
		for _, dir := range rootDirs {
			// Will auto skip any vendor directories found
			found, err := duat.FindGoDirs(dir, "TestMain")
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(-1)
			}
			dirs = append(dirs, found...)
		}
		rootDirs = dirs
	}

	for _, dir := range rootDirs {
		files, errGo := ioutil.ReadDir(dir)
		if errGo != nil {
			errs = append(errs,
				errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("dir", dir).With("rootDirs", rootDirs))
		}

		for _, file := range files {
			if file.IsDir() {
				continue
			}
			if strings.HasSuffix(file.Name(), "_test.go") {
				testDirs = append(testDirs, dir)
				break
			}
			// TODO Check for the test flag using the go AST, too heavy weight
			// for our purposes at this time
		}
	}

	// Now run go test in all of the the detected directories
	for _, dir := range testDirs {
		err := func() (err errors.Error) {
			cwd, errGo := os.Getwd()
			if errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}
			defer func() {
				if errGo = os.Chdir(cwd); errGo != nil {
					if err == nil {
						err = errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
					}
				}
			}()
			if errGo = os.Chdir(filepath.Join(cwd, dir)); errGo != nil {
				return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
			}

			// Introspect the system under test for CLI scenarios, if none then just do a single run
			allOpts, err := runner.GoGetConst(dir, "DuatTestOptions")
			if err != nil {
				return err
			}
			if allOpts == nil {
				return errors.New("could not find 'var DuatTestOptions [][]string'").With("var", "DuatTestOptions").With("stack", stack.Trace().TrimRuntime())
			}

			for _, appOpts := range allOpts {
				cliOpts := append(opts, appOpts...)
				if err = md.GoTest(map[string]string{}, tags, cliOpts); err != nil {
					return err
				}
			}
			return nil
		}()
		if err != nil {
			errs = append(errs, err)
		}
	}
	return outputs, errs
}

// dockerize is used to produce containers where appropriate within a build
// target directory.  Output is sent to the console as these steps can take
// very long periods of time and Travis with other build environments are
// prone to timeout if they see no output for an extended time.
//
func dockerize(md *duat.MetaData) (err errors.Error) {

	exists, _, err := md.ImageExists()

	pr, pw := io.Pipe()

	go func() {
		if !exists {
			err = md.ImageCreate(pw)
		}
		pw.Close()
	}()
	io.Copy(os.Stdout, pr)

	return err
}
