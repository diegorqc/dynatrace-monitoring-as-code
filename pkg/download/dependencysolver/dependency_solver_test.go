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

// +build unit

package dependencysolver

import (
	"testing"

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util"
	"gotest.tools/assert"
)

func TestProcessDownloadedFiles(t *testing.T) {
	fs := util.CreateTestFileSystem()
	//test for error
	badbasePath := "/test-resources/p1"
	err := ProcessDownloadedFiles(fs, badbasePath, "demo")
	assert.Error(t, err, "Not a valid path /test-resources/p1")
}

func TestGatherAndReplaceIds(t *testing.T) {
	fs := util.CreateTestFileSystem()
	basePath := "./test-resources/case1-alerting-profile"
	filesIds, err := gatherAndReplaceIds(fs, basePath)
	assert.NilError(t, err)
	entity1 := FileInfo{
		Name: "2058661211851054936",
		Path: "management-zone/supermg.json",
		Id:   "supermg.json"}
	assert.Equal(t, len(filesIds), 3)
	assert.Equal(t, filesIds[entity1.Name].Id, "supermg.json")
}
func TestReplaceDependencies(t *testing.T) {
	fs := util.CreateTestFileSystem()
	basePath := "./test-resources/case1-alerting-profile"
	filesIds := make(map[string]FileInfo)
	filesIds["2058661211851054936"] = FileInfo{
		Name: "super-mgzone",
		Id:   "2058661211851054936",
		Path: "management-zone/supermg"}
	filesIds["2058661211851054800"] = FileInfo{
		Name: "super-mgzone",
		Id:   "2058661211851054800",
		Path: "management-zone/supermg2"}

	depsForConfig, err := replaceDependencies(fs, basePath, filesIds)
	assert.NilError(t, err)
	assert.Equal(t, len(depsForConfig["alerting-profile"]), 2)
	assert.Equal(t, depsForConfig["alerting-profile"][0].Name, "managementZoneId")
	assert.Equal(t, depsForConfig["alerting-profile"][0].Value, "management-zone/supermg2")
	assert.Equal(t, depsForConfig["alerting-profile"][1].Name, "mzId")
	assert.Equal(t, depsForConfig["alerting-profile"][1].Value, "management-zone/supermg")

}

// func TestAddConfigsToYaml() {
// 	//fs := util.CreateTestFileSystem()
// 	p1 := DependencyConfig{
// 		Name:  "mzId",
// 		Value: "management-zone/supermg.id",
// 	}
// 	p2 := DependencyConfig{
// 		Name:  "managementZoneId",
// 		Value: "management-zone/supermg2.id",
// 	}
// 	data := make([]DependencyConfig, 0)
// 	data = append(data, p1)
// 	data = append(data, p2)

// 	//addConfigsToYaml(fs,files)
// }
