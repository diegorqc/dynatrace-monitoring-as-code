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

	"github.com/dynatrace-oss/dynatrace-monitoring-as-code/pkg/util"
	"github.com/spf13/afero"
	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

type FileInfo struct {
	Name string
	Id   string
	Path string
}

func ProcessDownloadedFiles(fs afero.IOFS, path string, envName string) error {
	filesIds, err := gatherAndReplaceIds(fs, path)
	if err != nil {
		return err
	}
	replaceDependencies(fs, path, filesIds)
	return nil
}

func gatherAndReplaceIds(fs afero.IOFS, basepath string) (filesIds map[string]FileInfo, err error) {
	filesIds = make(map[string]FileInfo)
	err = afero.Walk(fs.Fs, basepath, func(path string, info os.FileInfo, err error) error {

		if !info.IsDir() && strings.Contains(info.Name(), ".json") {

			file, err := afero.ReadFile(fs.Fs, path)
			if err != nil {
				util.Log.Error("error reading file %s", path)
				return err
			}

			entityId := gjson.GetBytes(file, "id")
			filesIds[entityId.String()] = FileInfo{
				Id:   entityId.String(),
				Name: info.Name(),
				Path: path,
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
		util.Log.Info("file " + path)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return filesIds, nil

}
func replaceDependencies(fs afero.IOFS, basepath string, filesIds map[string]FileInfo) error {

}
