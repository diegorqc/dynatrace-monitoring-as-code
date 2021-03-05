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

package files

import (
	"log"
	"os"
	"path/filepath"
	"regexp"

	"github.com/spf13/afero"
)

//FileManager is an interface to encapsulate the file read/creation process. It has 2 implementations
type FileManager interface {
	CreateFolder(path string) (fullpath string, err error)
	CreateFile(byteArray []byte, path string, name string, fileType string) (cleanName string, err error)
	ReadFile(path string) (content []byte, err error)
	ReadDir(path string) (dir []os.FileInfo, err error)
}

// fileManager implements interface FileManager
type fileManager struct {
	fileManager afero.Fs
}

//NewDiskFileManager creates a new FileManager instance with disk storage
func NewDiskFileManager() FileManager {
	el := &fileManager{}
	el.fileManager = afero.NewOsFs()
	return el
}

//CreateFolder creates a folder in the specified path
func (a *fileManager) CreateFolder(path string) (fullpath string, err error) {
	//path should be sanitized
	exist, err := afero.Exists(a.fileManager, path)
	if !exist && err == nil {
		err = a.fileManager.Mkdir(path, 0777)
	}
	if err != nil {
		return "", err
	}
	return path, nil
}

//CreateFile allows to write a file on disk using the specified path
func (a *fileManager) CreateFile(byteArray []byte, path string, name string, fileType string) (cleanName string, err error) {
	cleanName = sanitizeName(name)
	fullPath := filepath.Join(path, cleanName+fileType)
	err = afero.WriteFile(a.fileManager, fullPath, byteArray, 0664)
	if err != nil {
		return "", err
	}

	return cleanName, nil
}
func (a *fileManager) ReadFile(path string) (content []byte, err error) {
	file, err := afero.ReadFile(a.fileManager, path)
	if err != nil {
		return nil, err
	}
	return file, nil
}
func (a *fileManager) ReadDir(path string) (dir []os.FileInfo, err error) {
	dir, err = afero.ReadDir(a.fileManager, path)
	if dir != nil {
		return nil, err
	}
	return dir, nil
}

//NewInMemoryFileManager creates  a new instance of FileCreator that reads the files from disk and stores them in a virtual file tree
func NewInMemoryFileManager() FileManager {
	el := &fileManager{}
	el.fileManager = afero.NewMemMapFs()
	return el
}

//SanitizeName removes special characters, limits to max 50 characters in name, no special characters
func sanitizeName(name string) string {
	reg, err := regexp.Compile("[^a-zA-Z0-9-]+")
	if err != nil {
		log.Fatal(err)
	}
	processedString := reg.ReplaceAllString(name, "")
	runes := []rune(processedString)
	if len(runes) > 50 {
		processedString = string(runes[:50])
	}
	return processedString

}