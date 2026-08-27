package kustomize

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/librucha/krmgen/internal/tool"
	cons "github.com/librucha/krmgen/internal/utils"
	"gopkg.in/yaml.v3"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var allowedFileNames = map[string]any{"kustomization.yaml": nil, "kustomization.yml": nil, "kustomization": nil}

// runCommand is a seam: tests replace it to observe the kubectl invocation
// without running the binary.
var runCommand = tool.RunCommand

// FindKustomizeFile tries to find a kustomization file to build (see
// selectBuilder for which backend does the building). It returns an empty
// path and no error when the directory holds none, and an error when it
// holds more than one or cannot be walked.
func FindKustomizeFile(workDir string) (string, error) {
	var kustomizeFile string
	err := filepath.Walk(workDir, func(path string, info fs.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		_, ok := allowedFileNames[strings.ToLower(filepath.Base(path))]
		if ok {
			if kustomizeFile != "" {
				return fmt.Errorf("found multiple kustomization files under: %s", workDir)
			}
			kustomizeFile = path
		}
		return nil
	})
	if err != nil {
		return "", fmt.Errorf("search kustomize files failed. error: %w", err)
	}
	return kustomizeFile, nil
}

func BuildKustomize(kustomizeFile string, workDir string, resources string) (string, error) {
	if kustomizeFile == "" {
		return "", fmt.Errorf("no given kustomizeFile parameter")
	}
	var resourcesFile string
	if resources != "" {
		resourcesFile = filepath.Join(workDir, uuid.NewString()+".yml")
		err := os.WriteFile(resourcesFile, []byte(resources), cons.FilePerm)
		if err != nil {
			return "", fmt.Errorf("write file %qwith resources failed error: %w", resourcesFile, err)
		}
	}
	if err := prepareKustomizeFile(kustomizeFile, resourcesFile, workDir); err != nil {
		return "", err
	}

	return selectBuilder().Build(workDir)
}

func prepareKustomizeFile(kustomizeFile string, resourcesFile string, workDir string) error {

	// add resources to kustomize file
	var kustomizeFileYaml map[string]any
	fileContent, err := os.ReadFile(kustomizeFile)
	if err != nil {
		return fmt.Errorf("reading kustomization file %q failed error: %w", kustomizeFile, err)
	}

	err = yaml.Unmarshal(fileContent, &kustomizeFileYaml)
	if err != nil {
		return fmt.Errorf("unmarshaling kustomize file %q failed error: %w", kustomizeFile, err)
	}
	res, ok := kustomizeFileYaml["resources"]
	if !ok {
		res = []any{}
	}
	kustomizeResources, err := unwrapResources(res)
	if err != nil {
		return fmt.Errorf("unwraping resources from %q failed error: %w", kustomizeFile, err)
	}

	if resourcesFile != "" {
		relativePath, err := filepath.Rel(workDir, resourcesFile)
		if err != nil {
			relativePath = resourcesFile
		}
		kustomizeResources = append(kustomizeResources, relativePath)
		kustomizeFileYaml["resources"] = kustomizeResources
		updatedFileContent, err := yaml.Marshal(kustomizeFileYaml)
		if err != nil {
			return fmt.Errorf("marshaling updated file content failed error: %w", err)
		}
		err = os.WriteFile(kustomizeFile, updatedFileContent, cons.FilePerm)
		if err != nil {
			return fmt.Errorf("writing updated kustomize file %q failed error: %w", kustomizeFile, err)
		}
	}
	return nil
}

func unwrapResources(in any) ([]string, error) {
	collection, ok := in.([]any)
	if !ok {
		return nil, fmt.Errorf("given data should be type of %T but was %T", []any{}, in)
	}
	res := make([]string, len(collection))
	for i, item := range collection {
		s, ok := item.(string)
		if !ok {
			return nil, fmt.Errorf("item of given data should be type of %T but was %T", "", s)
		}
		res[i] = s
	}
	return res, nil
}
