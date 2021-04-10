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

package jsoncreator

import (
	"encoding/json"
	"fmt"
	"net/url"
	"path/filepath"
	"strings"

	"github.com/buger/jsonparser"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/api"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/rest"
	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util"
	"github.com/spf13/afero"
	"github.com/tidwall/sjson"
)

//go:generate mockgen -source=json_creator.go -destination=json_creator_mock.go -package=jsoncreator JSONCreator

//JSONCreator interface allows to mock the methods for unit testing
type JSONCreator interface {
	CreateJSONConfig(fs afero.Fs, client rest.DynatraceClient, api api.Api, value api.Value,
		path string) (name string, cleanName string, filter bool, err error)
	TransformJSONToMonacoFormat(fs afero.Fs, path string, basepath string, filename string, envName string) error
	ReplaceDependenciesInFile(fs afero.Fs, path string, filename string) error
	GetDependencies() map[string][]DependencyConfig
}
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

//JSONCreatorImp object
type JsonCreatorImp struct {
	ids          map[string]FileInfo
	dependencies map[string][]DependencyConfig
}

func (d *JsonCreatorImp) GetDependencies() map[string][]DependencyConfig {
	return d.dependencies
}

//NewJSONCreator creates a new instance of the jsonCreator
func NewJSONCreator() *JsonCreatorImp {
	result := JsonCreatorImp{}
	result.ids = make(map[string]FileInfo)
	result.dependencies = make(map[string][]DependencyConfig)
	return &result
}

//CreateJSONConfig creates a json file using the specified path and API data
func (d *JsonCreatorImp) CreateJSONConfig(fs afero.Fs, client rest.DynatraceClient, api api.Api, value api.Value,
	path string) (name string, cleanName string, filter bool, err error) {
	data, filter, err := getDetailFromAPI(client, api, value.Id)
	if err != nil {
		util.Log.Error("error getting detail %s from API", api.GetId())
		return "", "", false, err
	}
	if filter {
		return "", "", true, nil
	}
	jsonfile, name, cleanName, err := processJSONFile(data, value.Id, value.Name, api)
	if err != nil {
		util.Log.Error("error processing jsonfile %s", api.GetId())
		return "", "", false, err
	}
	fullPath := filepath.Join(path, cleanName+".json")
	err = afero.WriteFile(fs, fullPath, jsonfile, 0664)
	if err != nil {
		util.Log.Error("error writing detail %s", api.GetId())
		return "", "", false, err
	}
	return name, cleanName, false, nil
}

func getDetailFromAPI(client rest.DynatraceClient, api api.Api, name string) (dat map[string]interface{}, filter bool, err error) {

	name = url.QueryEscape(name)
	resp, err := client.ReadById(api, name)
	if err != nil {
		util.Log.Error("error getting detail for API %s", api.GetId(), name)
		return nil, false, err
	}
	err = json.Unmarshal(resp, &dat)
	if err != nil {
		util.Log.Error("error transforming %s from json to object", name)
		return nil, false, err
	}
	filter = isDefaultEntity(api.GetId(), dat)
	if filter {
		util.Log.Debug("Non-user-created default Object has been filtered out", name)
		return nil, true, err
	}
	return dat, false, nil
}

//processJSONFile removes and replaces properties for each json config to make them compatible with monaco standard
func processJSONFile(dat map[string]interface{}, id string, name string, api api.Api) ([]byte, string, string, error) {

	name, err := getNameForConfig(name, dat, api)
	if err != nil {
		return nil, "", "", err
	}
	cleanName := util.SanitizeName(name) //for using as the json filename
	jsonfile, err := json.MarshalIndent(dat, "", " ")

	if err != nil {
		util.Log.Error("error creating json file  %s", id)
		return nil, "", "", err
	}
	return jsonfile, name, cleanName, nil
}

//getNameForConfig return the correct name based on the type of config
func getNameForConfig(name string, dat map[string]interface{}, api api.Api) (string, error) {
	//for the apis that return a name for the config
	if name != "" {
		return name, nil
	}
	if api.GetId() == "reports" {
		return dat["dashboardId"].(string), nil
	}

	return "", fmt.Errorf("error getting name for config in api %q", api.GetId())
}

//isDefaultEntity returns if the object from the dynatrace API is readonly, in which case it shouldn't be downloaded
func isDefaultEntity(apiID string, dat map[string]interface{}) bool {

	switch apiID {
	case "dashboard":
		if dat["dashboardMetadata"] != nil {
			metadata := dat["dashboardMetadata"].(map[string]interface{})
			if metadata["preset"] != nil && metadata["preset"] == true {
				return true
			}
		}
		return false
	case "synthetic-location":
		if dat["type"] == "PRIVATE" {
			return false
		}
		return true
	case "synthetic-monitor":
		return false
	case "extension":
		return false
	case "aws-credentials":
		return false
	default:
		return false
	}
}

//replaceKeyProperties replaces key properties in each file and returns an object with the dependencies
func replaceKeyProperties(dat map[string]interface{}) map[string]interface{} {
	//removes id field
	//for applications
	if dat["identifier"] != nil {
		delete(dat, "identifier")
	}
	//the rest of configs
	if dat["id"] != nil {
		delete(dat, "id")
	}

	if dat["name"] != nil {
		dat["name"] = "{{.name}}"
	}

	if dat["displayName"] != nil {
		dat["displayName"] = "{{.name}}"
	}
	//for reports
	if dat["dashboardId"] != nil {
		dat["dashboardId"] = "{{.name}}"
	}

	return dat
}
func getKeywordId(configPath string) string {
	if strings.Contains(configPath, "application") {
		return "identifier"
	}
	return "id"
}
func (d *JsonCreatorImp) TransformJSONToMonacoFormat(fs afero.Fs, path string, basepath string, filename string, envName string) error {
	file, err := afero.ReadFile(fs, path)
	if err != nil {
		util.Log.Debug("error reading file %s", path)
		return err
	}
	basepath = strings.TrimRight(basepath, envName)
	configPath, err := filepath.Rel(basepath, path)
	if err != nil {
		util.Log.Debug("error getting relative path %s", path)
		return err
	}

	dat := make(map[string]interface{})
	err = json.Unmarshal(file, &dat)
	if err != nil {
		util.Log.Debug("error transforming %s from json to object", path)
		return err
	}
	keywordId := getKeywordId(configPath)
	entityId, exist := dat[keywordId]
	if !exist {
		util.Log.Debug("error finding id %s in %s", configPath, path)
		return err
	}
	sentityId, valid := entityId.(string)
	if !valid {
		util.Log.Debug("error transforming id %s in %s", configPath, path)
		return err
	}
	d.ids[sentityId] = FileInfo{
		Id:   sentityId,
		Name: filename,
		Path: configPath,
	}
	dat = replaceKeyProperties(dat)

	if err != nil {
		util.Log.Debug("error deleting ids %s", path)
		return err
	}
	jsonfile, err := json.MarshalIndent(dat, "", " ")
	err = afero.WriteFile(fs, path, jsonfile, 0664)

	if err != nil {
		util.Log.Debug("error writing file %s", path)
		return err
	}
	return nil
}

func (d *JsonCreatorImp) ReplaceDependenciesInFile(fs afero.Fs, path string, filename string) error {
	file, err := afero.ReadFile(fs, path)
	if err != nil {
		util.Log.Error("error reading file %s", path)
		return err
	}
	configName := filepath.Base(filepath.Dir(path))

	var keys []string
	file, dependenciesToAdd := replaceDependenciesInJson(filename, file, path, d, keys, d.dependencies[configName])
	if api.IsApi(configName) && dependenciesToAdd != nil {
		d.dependencies[configName] = append(d.dependencies[configName], dependenciesToAdd...)
	}
	err = afero.WriteFile(fs, path, file, 0664)
	if err != nil {
		return err
	}
	return nil
}

//replaceDependenciesInJson will search the json file for keywords. If found any, will look for the id in the filesIds list and replace it with the relative path.
func replaceDependenciesInJson(filename string, file []byte, path string, d *JsonCreatorImp, keys []string, parametersYaml []DependencyConfig) ([]byte, []DependencyConfig) {
	pathsToSet := make(map[string]string)

	err := jsonparser.ObjectEach(file, func(key []byte, value []byte, dataType jsonparser.ValueType, offset int) error {
		if dataType.String() == "string" || dataType.String() == "number" {
			configName := filepath.Base(filepath.Dir(path))
			pathsToSet, parametersYaml, _ = findDependency(filename, pathsToSet, parametersYaml, d.ids, string(key), string(value), configName, keys)

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

	if isKeywordDependency(key) {
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

func isKeywordDependency(keyword string) bool {
	switch keyword {
	case
		"mzId",
		"managementZoneId",
		"applicationIdentifier":
		return true
	}
	return false
}
