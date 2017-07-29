package runner

// This file contains the implementations of various functions for accessing
// and using google firebase.  Firebase is being used by TensorFlow Studio
// to contextual information about tasks it has requested be executed, via
// Google PubSub

import (
	"flag"
	"fmt"
	"io/ioutil"
	"os"
	"time"

	"github.com/zabawaba99/firego"
	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"

	"github.com/davecgh/go-spew/spew"
)

var (
	authDB = flag.String("firebase-account-file", "", "The file in which the Google Service Account authrization details are stored for Firebase")
)

func init() {
	*authDB = os.Getenv("HOME") + "/.ssh/google-firebase-auth.json"
}

type FirebaseDB struct {
	fb        *firego.Firebase
	projectID string
}

func NewDatabase(projectID string) (db *FirebaseDB, err error) {

	info, err := os.Stat(*authDB)
	if err != nil {
		return nil, err
	}
	if 0600 != info.Mode() {
		return nil, fmt.Errorf(`file permissions for %s are too liberal, permissions should be 0600, 
		use the shell command 'chmod 0600 %s' to fix this`, *authDB, *authDB)
	}

	d, err := ioutil.ReadFile(*authDB)
	if err != nil {
		return nil, err
	}

	conf, err := google.JWTConfigFromJSON(d, "https://www.googleapis.com/auth/userinfo.email",
		"https://www.googleapis.com/auth/firebase.database")
	if err != nil {
		return nil, err
	}

	db = &FirebaseDB{
		projectID: projectID,
	}

	db.fb = firego.New(fmt.Sprintf("https://%s.firebaseio.com", projectID), conf.Client(oauth2.NoContext))

	firego.TimeoutDuration = 5 * time.Second

	return db, nil
}

func (fb *FirebaseDB) GetAll() (result string, err error) {

	v := map[string]interface{}{}

	fb.fb.Shallow(true)
	fb.fb.fb.Child("users").Value(&v)

	return spew.Sdump(v), nil
}
