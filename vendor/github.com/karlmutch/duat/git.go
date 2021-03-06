package duat

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/jjeffery/kv"     // Forked copy of https://github.com/jjeffery/kv
	"github.com/go-stack/stack" // Forked copy of https://github.com/go-stack/stack

	"gopkg.in/src-d/go-git.v4" // Not forked due to depency tree being too complex, src-d however are a serious org so I dont expect the repo to disappear
	"gopkg.in/src-d/go-git.v4/plumbing"
)

// This file contains some utility functions for extracting and using git information

func (md *MetaData) LoadGit(dir string, scanParents bool) (err kv.Error) {

	if md.Git != nil {
		return kv.NewError("git info already loaded, set Git member to nil if new information desired").With("stack", stack.Trace().TrimRuntime())
	}

	gitDir, errGo := filepath.Abs(dir)
	if errGo != nil {
		md.Git.Err = kv.Wrap(errGo, "directory could not be resolved").With("dir", dir).With("stack", stack.Trace().TrimRuntime()).With("git", gitDir)
		return md.Git.Err
	}

	for {
		if _, errGo = os.Stat(filepath.Join(gitDir, ".git")); errGo == nil {
			break
		}
		if !scanParents {
			return kv.Wrap(errGo, "does not appear to be the top directory of a git repo").With("stack", stack.Trace().TrimRuntime()).With("git", gitDir)
		}
		gitDir = filepath.Dir(gitDir)
		if len(gitDir) < 2 {
			return kv.Wrap(errGo, "could not locate a git repo in the directory heirarchy").With("stack", stack.Trace().TrimRuntime()).With("dir", dir)
		}
	}

	md.Git = &GitInfo{
		Dir: gitDir,
	}

	repo, errGo := git.PlainOpen(gitDir)
	if errGo != nil {
		md.Git.Err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("git", gitDir)
		return md.Git.Err
	}
	ref, errGo := repo.Head()
	if errGo != nil {
		md.Git.Err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("git", gitDir)
		return md.Git.Err
	}

	splits := strings.Split(ref.Name().String(), "/")

	// If we are detached there might not be a branch for us to use
	if len(splits) > 2 {
		//Scoop up everything after the refs/heads/ to get the branch name
		//and reattach any slashes we took out
		md.Git.Branch = strings.Join(splits[2:], "/")
	} else {
		// The branch might be available through Travis, if so and it is not available elsewhere
		// use that value
		md.Git.Branch = os.Getenv("TRAVIS_BRANCH")
	}

	md.Git.Repo = repo
	refs, errGo := repo.Remotes()
	if errGo != nil {
		md.Git.Err = kv.Wrap(errGo).With("stack", stack.Trace().TrimRuntime()).With("git", gitDir).With("repo", repo)
		return md.Git.Err
	}

	urlLoc := refs[0].Config().URLs[0]
	if strings.HasPrefix(urlLoc, "git@github.com:") {
		urlLoc = strings.Replace(urlLoc, "git@github.com:", "https://github.com/", 1)
	}
	gitURL, errGo := url.Parse(urlLoc)
	if errGo != nil {
		md.Git.Err = kv.Wrap(errGo).With("url", urlLoc).With("git", gitDir).With("stack", stack.Trace().TrimRuntime())
		return md.Git.Err
	}
	md.Git.URL = *gitURL

	// Now try to find the first tag that matches the current HEAD
	head, _ := md.Git.Repo.Head()
	md.Git.Hash = head.Hash().String()

	tags, _ := md.Git.Repo.Tags()
	_ = tags.ForEach(func(t *plumbing.Reference) error {
		if head.Hash() == t.Hash() {
			splits := strings.Split(t.Name().String(), "/")
			md.Git.Tag = splits[len(splits)-1]
		}
		return nil
	})

	return nil
}
