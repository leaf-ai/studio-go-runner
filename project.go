package runner

// This file contains the marshalling for TFStudio project meta-data retrieved
// from the TFStudio DB backend

import "encoding/json"

func UnmarshalTFSMetaData(data []byte) (root *TFSMetaData, err error) {

	root = &TFSMetaData{}

	err = json.Unmarshal(data, root)
	return root, err
}

func (r *TFSMetaData) Marshal() ([]byte, error) {
	return json.Marshal(r)
}

type Artifacts struct {
	Modeldir  Modeldir `json:"modeldir"`
	Output    Modeldir `json:"output"`
	Tb        Modeldir `json:"tb"`
	Workspace Modeldir `json:"workspace"`
}

type TFSMetaData struct {
	Args               []string        `json:"args"`
	Artifacts          Artifacts       `json:"artifacts"`
	Filename           string          `json:"filename"`
	Key                string          `json:"key"`
	Owner              string          `json:"owner"`
	Pythonenv          []string        `json:"pythonenv"`
	ResourcesNeeded    ResourcesNeeded `json:"resources_needed"`
	Status             string          `json:"status"`
	TimeAdded          float64         `json:"time_added"`
	TimeLastCheckpoint float64         `json:"time_last_checkpoint"`
}

type Modeldir struct {
	Key     string `json:"key"`
	Local   string `json:"local"`
	Mutable bool   `json:"mutable"`
}

type ResourcesNeeded struct {
	Cpus float64 `json:"cpus"`
	Gpus float64 `json:"gpus"`
	Hdd  string  `json:"hdd"`
	Ram  string  `json:"ram"`
}
