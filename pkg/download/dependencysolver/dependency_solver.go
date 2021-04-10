// @license
// Copyright 2021 Dynatrace LLC
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package dependencysolver

import (
	"os"
	"strings"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download/jsoncreator"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/download/yamlcreator"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
)

//go:generate mockgen -source=dependency_solver.go -destination=dependency_solver_mock.go -package=dependencysolver DependencySolver

type DependencySolver interface {
	ProcessDownloadedFiles(fs afero.Fs, path string, envName string) error
}
type DependencySolverImp struct{}

//NewJSONCreator creates a new instance of the jsonCreator
func NewDependencySolver() *DependencySolverImp {
	result := DependencySolverImp{}
	return &result
}

//ProcessDownloadedFiles executes a 3 step process to locate and replace dependencies with relative paths
func (d *DependencySolverImp) ProcessDownloadedFiles(fs afero.Fs, jcreator jsoncreator.JSONCreator, ycreator yamlcreator.YamlCreator,
	path string, envName string) error {
	validPath, err := afero.Exists(fs, path)
	if !validPath {
		return errors.Errorf("Not a valid path %s", path)
	}
	err = gatherAndReplaceIds(fs, jcreator, path, envName)
	if err != nil {
		util.Log.Error("error while replacing ids for downloaded configs")
		return err
	}
	err = replaceDependencies(fs, path, jcreator)
	if err != nil {
		util.Log.Error("error while replacing dependencies for downloaded configs")
		return err
	}
	err = addConfigsToYaml(fs, path, jcreator, ycreator)
	if err != nil {
		util.Log.Error("error while adding dependencies to yaml files")
		return err
	}
	return nil
}

//gatherAndReplaceIds gathers the ids and paths for all the downloaded configs and then deletes them from the files
func gatherAndReplaceIds(fs afero.Fs, jcreator jsoncreator.JSONCreator, basepath string, envName string) (err error) {

	err = afero.Walk(fs, basepath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.Contains(info.Name(), ".json") {
			jcreator.TransformJSONToMonacoFormat(fs, path, basepath, info.Name(), envName)
			if err != nil {
				util.Log.Error("error transforming json %s", err)
				return err
			}
		}
		return nil
	})
	if err != nil {
		util.Log.Error("error reading the directory structure %s", err)
		return err
	}
	return nil
}

//replaceDependencies uses the ids from  the previous step and looks for the same values inside the json properties
func replaceDependencies(fs afero.Fs, basepath string, jcreator jsoncreator.JSONCreator) error {

	err := afero.Walk(fs, basepath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.Contains(info.Name(), ".json") {
			err = jcreator.ReplaceDependenciesInFile(fs, path, info.Name())
			if err != nil {
				util.Log.Error("error while replacing dependencies in file %s detail: %s", path, err)
			}
		}
		return nil
	})
	if err != nil {
		util.Log.Error("error reading the directory structure %s", err)
		return err
	}
	return nil
}

//addConfigs adds the dependencies to the corresponding yaml file
func addConfigsToYaml(fs afero.Fs, basepath string, jcreator jsoncreator.JSONCreator, ycreator yamlcreator.YamlCreator) error {
	//for each element in the map, find the yaml config and append the array of new configs and save
	configs := jcreator.GetDependencies()
	err := afero.Walk(fs, basepath, func(path string, info os.FileInfo, err error) error {

		if !info.IsDir() && strings.Contains(info.Name(), ".yaml") {
			err = ycreator.AddDependencies(fs, path, configs)
			if err != nil {
				util.Log.Error("error while adding dependencies to file %s", path)
				return err
			}

		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}
