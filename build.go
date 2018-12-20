// +build ignore

package main

import (
	"flag"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"

	runner "github.com/leaf-ai/studio-go-runner/internal/runner"

	"github.com/karlmutch/duat"
	"github.com/karlmutch/duat/version"
	logxi "github.com/karlmutch/logxi/v1"

	"github.com/karlmutch/errors" // Forked copy of https://github.com/jjeffery/errors
	"github.com/karlmutch/stack"  // Forked copy of https://github.com/go-stack/stack

	"github.com/karlmutch/envflag" // Forked copy of https://github.com/GoBike/envflag

	"gopkg.in/src-d/go-license-detector.v2/licensedb"
	"gopkg.in/src-d/go-license-detector.v2/licensedb/filer"
)

var (
	logger = logxi.New("build.go")

	verbose     = flag.Bool("v", false, "When enabled will print internal logging for this tool")
	recursive   = flag.Bool("r", false, "When enabled this tool will visit any sub directories that contain main functions and build in each")
	userDirs    = flag.String("dirs", ".", "A comma separated list of root directories that will be used a starting points looking for Go code, this will default to the current working directory")
	imageOnly   = flag.Bool("image-only", false, "Used to start at the docker build step, will progress to github release, if not set the build halts after compilation")
	githubToken = flag.String("github-token", "", "If set this will automatically trigger a release of the binary artifacts to github at the current version")
	buildLog    = flag.String("runner-build-log", "", "The location of the build log used by the invoking script, to be uploaded to github")
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
	execDirs := []string{}

	err := errors.New("")

	// If this is a recursive build scan all inner directories looking for go code.
	// Skip the vendor directory and when looking for code examine to see if it is
	// test code, or go generate style code.
	//
	// Build any code that is found and respect the go generate first.
	//
	if *recursive {
		for _, dir := range rootDirs {
			// Dont allow the vendor directory to creep in
			if filepath.Base(dir) == "vendor" {
				continue
			}

			// Otherwise look for meanful code that can be run either as tests
			// or as a standard executable
			// Will auto skip any vendor directories found
			execDirs, err = duat.FindGoDirs(dir, []string{"main", "TestMain"})
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(-1)
			}
		}
	} else {
		execDirs = rootDirs
	}

	// Now remove duplicates within the list of directories that we can potentially
	// visit during builds, removing empty strings
	{
		lastSeen := ""
		deDup := make([]string, 0, len(execDirs))
		sort.Strings(execDirs)
		for _, dir := range execDirs {
			if dir != lastSeen {
				deDup = append(deDup, dir)
				lastSeen = dir
			}
		}
		execDirs = deDup
	}

	allLics, err := licenses(".")
	if err != nil {
		logger.Warn(errors.Wrap(err, "could not create a license manifest").With("stack", stack.Trace().TrimRuntime()).Error())
	}
	licf, errGo := os.OpenFile("licenses.manifest", os.O_WRONLY|os.O_CREATE, 0644)
	if errGo != nil {
		logger.Warn(errors.Wrap(errGo, "could not create a license manifest").With("stack", stack.Trace().TrimRuntime()).Error())
	} else {
		for dir, lics := range allLics {
			licf.WriteString(fmt.Sprint(dir, ",", lics[0].lic, ",", lics[0].score, "\n"))
		}
		licf.Close()
	}

	// Invoke the generator in any of the root dirs and their desendents without
	// looking for a main for TestMain as generated code can exist throughout any
	// of our repos packages
	if outputs, err := runGenerate(rootDirs, "README.md"); err != nil {
		for _, aLine := range outputs {
			logger.Info(aLine)
		}
		logger.Warn(err.Error())
		os.Exit(-3)
	}

	// Take the discovered directories and build them from a deduped
	// directory set
	for _, dir := range execDirs {
		if outputs, err := runBuild(dir, "README.md"); err != nil {
			for _, aLine := range outputs {
				logger.Info(aLine)
			}
			logger.Warn(err.Error())
			os.Exit(-4)
		}
	}

	outputs := []string{}

	if err == nil {
		for _, dir := range execDirs {
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
		os.Exit(-5)
	}
}

type License struct {
	lic   string
	score float32
}

// licenses returns a list of directories and files that have license and confidences related to
// each.  An attempt is made to rollup results so that directories with licenses that match all
// files are aggregated into a single entry for the items, any small variations for files are
// called out and left in the output.  Also directories are rolled up where their children match.
//
func licenses(dir string) (lics map[string][]License, err errors.Error) {
	lics = map[string][]License{}
	errGo := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() {
			return nil
		}
		if len(path) > 1 && path[0] == '.' {
			return filepath.SkipDir
		}
		fr, errGo := filer.FromDirectory(path)
		if errGo != nil {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		licenses, errGo := licensedb.Detect(fr)
		if errGo != nil && errGo.Error() != "no license file was found" {
			return errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
		}
		if len(licenses) == 0 {
			return nil
		}
		if _, isPresent := lics[path]; !isPresent {
			lics[path] = []License{}
		}
		for lic, conf := range licenses {
			lics[path] = append(lics[path], License{lic: lic, score: conf})
		}
		sort.Slice(lics[path], func(i, j int) bool { return lics[path][i].score < lics[path][j].score })
		return nil
	})
	if errGo != nil {
		return nil, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime())
	}
	return lics, nil
}

// runGenerate is used to do a stock go generate within our project directories
//
func runGenerate(dirs []string, verFn string) (outputs []string, err errors.Error) {

	for _, dir := range dirs {
		files, err := duat.FindGoGenerateDirs([]string{dir}, []string{})
		if err != nil {
			fmt.Fprintln(os.Stderr, err.Error())
			os.Exit(-6)
		}

		genFiles := make([]string, 0, len(files))
		for _, fn := range files {

			// Will skip any vendor directories found
			if strings.Contains(fn, "/vendor/") || strings.HasSuffix(fn, "/vendor") {
				continue
			}

			genFiles = append(genFiles, fn)
		}

		outputs, err = func(dir string, verFn string) (outputs []string, err errors.Error) {
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
			md, err := duat.NewMetaData(cwd, verFn)
			if err != nil {
				return outputs, err
			}

			if outputs, err = generate(md, genFiles); err != nil {
				return outputs, err
			}
			return outputs, nil
		}(dir, verFn)
		if err != nil {
			return outputs, err.With("dir", dir)
		}
	}
	return outputs, nil
}

// runBuild is used to restore the current working directory after the build itself
// has switched directories
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
		return outputs, err
	}

	// Testing first will speed up failing in the event of a compiler or functional issue
	if err == nil && !*imageOnly {
		logger.Info(fmt.Sprintf("testing %s", dir))
		out, errs := test(md)
		outputs = append(outputs, out...)
		if len(errs) != 0 {
			return outputs, errs[0]
		}
	}

	// If we are in a container then do a stock compile, if not then it is
	// time to dockerize all the things
	if len(runtime) != 0 {
		logger.Info(fmt.Sprintf("building %s", dir))
		outputs, err = build(md)
		if err != nil {
			return outputs, err
		}
	}

	if len(runtime) == 0 {
		// Dont Dockerize in the main root directory of a project.  The root
		// dir Dockerfile is for a projects build container typically.
		if dir != "." {
			logger.Info(fmt.Sprintf("dockerizing %s", dir))
			if err = dockerize(md); err != nil {
				return outputs, err
			}
			// Check for a bin directory and continue if none
			if _, errGo := os.Stat("./bin"); errGo == nil {
				outputs, err = md.GoFetchBuilt()
			}
		}
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

		// Add to the release a build log if one was being generated
		if len(*buildLog) != 0 && len(outputs) != 0 {
			log, errGo := filepath.Abs(filepath.Join(cwd, *buildLog))
			if errGo == nil {
				// sync the filesystem, blindly
				cmd := exec.Command("sync")
				// Wait for it to stop and ignore the result
				_ = cmd.Run()
				if fi, errGo := os.Stat(log); errGo == nil {
					if fi.Size() > 0 {
						outputs = append(outputs, log)
					} else {
						logger.Warn(errors.New("empty log").With("log", log).With("stack", stack.Trace().TrimRuntime()).Error())
					}
				} else {
					logger.Warn(errors.Wrap(errGo).With("log", log).With("stack", stack.Trace().TrimRuntime()).Error())
				}
			} else {
				logger.Warn(errors.Wrap(errGo).With("log", log).With("stack", stack.Trace().TrimRuntime()).Error())
			}
		}

		if len(outputs) != 0 && !*imageOnly {
			logger.Info(fmt.Sprintf("github releasing %s", outputs))
			err = md.CreateRelease(*githubToken, "", outputs)
		}
	}

	if len(runtime) == 0 {
		return outputs, err
	}

	// Now work on the AWS push

	// Now do the Azure push

	return outputs, err
}

func generate(md *duat.MetaData, files []string) (outputs []string, err errors.Error) {
	for _, file := range files {
		logger.Info("generating " + file)
		osEnv := os.Environ()
		env := make(map[string]string, len(osEnv))
		for _, evar := range os.Environ() {
			pair := strings.SplitN(evar, "=", 2)
			env[pair[0]] = pair[1]
		}
		if outputs, err = md.GoGenerate(file, env, []string{}, []string{}); err != nil {
			return outputs, err
		}
	}
	return nil, nil
}

// build performs the default build for the component within the directory specified, but does
// no further than producing binaries that need to be done within a isolated container
//
func build(md *duat.MetaData) (outputs []string, err errors.Error) {

	// Before beginning purge the bin directory into which our files are saved
	// for downstream packaging etc
	//
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
	// Get any default directories from the linux env var that is used for shared libraries
	libPaths := strings.Split(os.Getenv("LD_LIBRARY_PATH"), ":")
	filepath.Walk("/usr/lib", func(path string, info os.FileInfo, err error) error {
		if info.IsDir() {
			libPaths = append(libPaths, path)
		}
		return nil
	})
	for _, aPath := range libPaths {
		if _, errGo := os.Stat(filepath.Join(aPath, "libcuda.so.1")); errGo == nil {
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

func k8sPod() (isPod bool, err errors.Error) {

	fn := "/proc/self/mountinfo"

	contents, errGo := ioutil.ReadFile(fn)
	if errGo != nil {
		return false, errors.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("file", fn)
	}
	for _, aMount := range strings.Split(string(contents), "\n") {
		fields := strings.Split(aMount, " ")
		// For information about the individual fields c.f. https://www.kernel.org/doc/Documentation/filesystems/proc.txt
		if len(fields) > 5 {
			if fields[4] == "/run/secrets/kubernetes.io/serviceaccount" {
				return true, nil
			}
		}
	}
	return false, nil
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
	tags := []string{}

	// Look for the Kubernetes is present indication and disable
	// tests if it is not
	sPod, _ := k8sPod()
	if !sPod {
		opts = append(opts, "-test.short")
	} else {
		opts = append(opts, "-test.timeout=15m")
		opts = append(opts, "-test.run=TestÄE2EMetadataMultiPassRun")
		opts = append(opts, "--use-k8s")
	}

	if !GPUPresent() {
		// Look for GPU Hardware and set the build flags for the tests based
		// on its presence
		tags = append(tags, "NO_CUDA")
		opts = append(opts, "--no-gpu")
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
			found, err := duat.FindGoDirs(dir, []string{"TestMain"})
			if err != nil {
				fmt.Fprintln(os.Stderr, err.Error())
				os.Exit(-7)
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
