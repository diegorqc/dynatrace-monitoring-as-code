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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/api"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util"
	"github.com/pkg/errors"
	"github.com/spf13/afero"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
	"gopkg.in/yaml.v3"
)

type FileInfo struct {
	Name string
	Id   string
	Path string
}
type DependencyConfig struct {
	Name     string
	Value    string
	JsonName string
}

//ProcessDownloadedFiles executes a 3 step process to locate and replace dependencies with relative paths
func ProcessDownloadedFiles(fs afero.IOFS, path string, envName string) error {
	validPath, err := afero.Exists(fs.Fs, path)
	if !validPath {
		return errors.Errorf("Not a valid path %s", path)
	}
	filesIds, err := gatherAndReplaceIds(fs, path)
	if err != nil {
		util.Log.Error("error while replacing ids for downloaded configs")
		return err
	}
	configsToAdd, err := replaceDependencies(fs, path, filesIds)
	if err != nil {
		return err
	}
	err = addConfigs(fs, path, configsToAdd)
	if err != nil {
		return err
	}
	return nil
}

//gatherAndReplaceIds gathers the ids and paths for all the downloaded configs and then deletes them from the files
func gatherAndReplaceIds(fs afero.IOFS, basepath string) (filesIds map[string]FileInfo, err error) {
	filesIds = make(map[string]FileInfo)

	err = afero.Walk(fs.Fs, basepath, func(path string, info os.FileInfo, err error) error {

		if !info.IsDir() && strings.Contains(info.Name(), ".json") {

			file, err := afero.ReadFile(fs.Fs, path)
			if err != nil {
				util.Log.Error("error reading file %s", path)
				return err
			}
			relativePath, err := filepath.Rel(basepath, path)
			if err != nil {
				util.Log.Error("error getting relative path %s", path)
				return err
			}
			//this will fetch the id in the first level of the json config
			getIdKeyword(relativePath)
			entityId := gjson.GetBytes(file, "id")
			filesIds[entityId.String()] = FileInfo{
				Id:   entityId.String(),
				Name: info.Name(),
				Path: relativePath,
			}
			file, err = sjson.DeleteBytes(file, "id")
			if err != nil {
				util.Log.Error("error deleting ids %s", path)
				return err
			}

			err = afero.WriteFile(fs.Fs, path, file, 0664)

			if err != nil {
				util.Log.Error("error writing file %s", path)
				return err
			}
		}
		return nil
	})
	if err != nil {
		util.Log.Error("error reading the directory structure %s", err)
		return nil, err
	}
	return filesIds, nil
}
func getIdKeyword(path string) string {
	if strings.Contains(path, "application") {
		return "identifier"
	} else {
		return "id"
	}
}

//replaceDependencies uses the ids from  the previous step and looks for the same values inside the json properties
func replaceDependencies(fs afero.IOFS, basepath string, filesIds map[string]FileInfo) (map[string][]DependencyConfig, error) {
	configsToAdd := make(map[string][]DependencyConfig)

	err := afero.Walk(fs.Fs, basepath, func(path string, info os.FileInfo, err error) error {
		if !info.IsDir() && strings.Contains(info.Name(), ".json") {
			file, err := afero.ReadFile(fs.Fs, path)
			if err != nil {
				util.Log.Error("error reading file %s", path)
				return err
			}
			configName := filepath.Base(filepath.Dir(path))

			var keys []string
			file, dependenciesToAdd := replaceDependenciesInJson(info.Name(), file, path, filesIds, keys, configsToAdd[configName])
			if api.IsApi(configName) && dependenciesToAdd != nil {
				configsToAdd[configName] = append(configsToAdd[configName], dependenciesToAdd...)
			}
			err = afero.WriteFile(fs.Fs, path, file, 0664)
			if err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		util.Log.Error("error reading the directory structure %s", err)
		return nil, err
	}
	return configsToAdd, nil
}

//replaceDependenciesInJson will search the json file for keywords. If found any, will look for the id in the filesIds list and replace it with the relative path.
func replaceDependenciesInJson(filename string, file []byte, path string, filesIds map[string]FileInfo, keys []string, parametersYaml []DependencyConfig) ([]byte, []DependencyConfig) {
	pathsToSet := make(map[string]string)

	err := jsonparser.ObjectEach(file, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if dataType.String() == "string" || dataType.String() == "number" {
			configName := filepath.Base(filepath.Dir(path))
			pathsToSet, parametersYaml, _ = findDependency(filename, pathsToSet, parametersYaml, filesIds, string(key), string(value), configName, keys)

		} else {
			//Value might be nested
		}
		return nil
	}, keys...)

	if err != nil {
		util.Log.Error("error while reading the json file %s", path)
	}
	//replaces the values in the actual file base on the list
	for key, value := range pathsToSet {
		file, err = sjson.SetBytes(file, key, value)
		if err != nil {
			util.Log.Error("error setting json values %s", path, file)
		}
	}
	return file, parametersYaml
}
func findDependency(filename string, pathsToSet map[string]string, parametersYaml []DependencyConfig,
	filesIds map[string]FileInfo, key string, value string, configName string, keys []string) (map[string]string, []DependencyConfig, error) {

	if IsKeywordDependency(key) {
		parent, existId := filesIds[value]
		if existId {
			pathForJson := strings.Join(keys[:], ".") + string(key)
			pathsToSet[pathForJson] = "{{." + string(key) + "}}"
			//get api id
			filename = strings.Replace(filename, ".json", "", -1)
			pathToDependency := strings.Replace(parent.Path, ".json", ".id", 1)
			if api.IsApi(configName) {
				parameter := DependencyConfig{
					Name:     string(key),
					Value:    pathToDependency,
					JsonName: filename}
				parametersYaml = append(parametersYaml, parameter)
			}
			return pathsToSet, parametersYaml, nil
		} else {
			return pathsToSet, parametersYaml, fmt.Errorf("Configuration %s has a dependency for the field %s, but the Id %s is not found", configName, string(key), string(value))
		}
	}
	return pathsToSet, parametersYaml, nil
}

func IsKeywordDependency(keyword string) bool {
	switch keyword {
	case
		"mzId",
		"managementZoneId",
		"applicationIdentifier":
		return true
	}
	return false
}

//addConfigs adds the dependencies to the corresponding yaml file
func addConfigs(fs afero.IOFS, basepath string, dependencies map[string][]DependencyConfig) error {
	//for each element in the map, find the yaml config and append the array of new configs and save
	err := afero.Walk(fs.Fs, basepath, func(path string, info os.FileInfo, err error) error {

		if !info.IsDir() && strings.Contains(info.Name(), ".yaml") {

			configName := filepath.Base(filepath.Dir(path))
			yamlParameters, exist := dependencies[configName]
			if exist {
				file, err := afero.ReadFile(fs.Fs, path)
				if err != nil {
					util.Log.Error("error reading file %s", path)
					return err
				}
				file, err = addConfigsToYaml(file, yamlParameters)
				if err != nil {
					util.Log.Error("error setting configs in yaml %s", path)
					return err
				}
				err = afero.WriteFile(fs.Fs, path, file, 0664)
				if err != nil {
					util.Log.Error("error writing configs in yaml %s", path)
					return err
				}
			}

		}
		return nil
	})
	if err != nil {
		return err
	}
	return nil
}

func addConfigsToYaml(file []byte, parameters []DependencyConfig) ([]byte, error) {

	var yc2 = make(map[string]interface{})
	if err := yaml.Unmarshal(file, &yc2); err != nil {
		return nil, err
	}
	for _, parameter := range parameters {

		subValues, exist := yc2[parameter.JsonName]
		if exist {
			castValues := subValues.([]interface{})
			par := make(map[string]string)
			par[parameter.Name] = parameter.Value
			castValues = append(castValues, par)
			//castValues[parameter.Name] = parameter.Value
			yc2[parameter.JsonName] = castValues

			file, err := yaml.Marshal(yc2)
			if err != nil {
				util.Log.Error("error parsing yaml file: %v", err)
				return nil, err
			}
			return file, nil
		}

	}

	return file, nil
}
