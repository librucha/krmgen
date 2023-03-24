package kustomize

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/librucha/krmgen/internal/template"
	"github.com/librucha/krmgen/internal/tool"
	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v3"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var allowedFileNames = map[string]any{"kustomization.yaml": nil, "kustomization.yml": nil, "kustomization": nil}

// FindKustomizeFile try to find files usable for 'kubectl kustomize' command.
// Returns founded kustomization file path.
func FindKustomizeFile(workDir string) string {
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
				log.Fatalf("found multiple kustomization files under: %s", workDir)
			}
			kustomizeFile = path
		}
		return nil
	})
	if err != nil {
		log.Fatalf("search kustomize files failed. error: %s", err)
	}
	return kustomizeFile
}

func BuildKustomize(kustomizeFile string, workDir string, resources string) string {
	if kustomizeFile == "" {
		log.Fatalf("no given kustomizeFile parameter")
	}
	var resourcesFile string
	if resources != "" {
		resourcesFile = filepath.Join(workDir, uuid.NewString()+".yml")
		err := os.WriteFile(resourcesFile, []byte(resources), os.ModePerm)
		if err != nil {
			log.Fatalf("write file %qwith resources failed error: %s", resourcesFile, err)
		}
	}
	prepareKustomizeFile(kustomizeFile, resourcesFile, workDir)

	args := []string{
		"kustomize",
		workDir,
	}
	stdOut, stdErr, err := tool.RunCommand("kubectl", args...)
	if err != nil {
		log.Fatalf("run kubectl kustomize failed error: %s reason: %s", err, stdErr)
	}
	return stdOut
}

func prepareKustomizeFile(kustomizeFile string, resourcesFile string, workDir string) {

	// backup kustomize file
	backupFile(kustomizeFile)
	// evaluate templates
	evaluateTemplates(kustomizeFile)

	// add resources to kustomize file
	var kustomizeFileYaml map[string]any
	fileContent, err := os.ReadFile(kustomizeFile)
	if err != nil {
		log.Fatalf("reading kustomization file %q failed error: %s", kustomizeFile, err)
	}

	err = yaml.Unmarshal(fileContent, &kustomizeFileYaml)
	if err != nil {
		log.Fatalf("unmarshaling kustomize file %q failed error: %s", kustomizeFile, err)
	}
	res, ok := kustomizeFileYaml["resources"]
	if !ok {
		res = []any{}
	}
	kustomizeResources, err := unwrapResources(res)
	if err != nil {
		log.Fatalf("unwraping resources from %q failed error: %s", kustomizeFile, err)
	}

	for _, resourceFile := range kustomizeResources {
		if !strings.HasPrefix(resourceFile, "http") {
			backupFile(resourceFile)
			evaluateTemplates(resourceFile)
		}
	}

	patches, ok := kustomizeFileYaml["patchesStrategicMerge"]
	if !ok {
		patches = []any{}
	}
	kustomizePatches, err := unwrapResources(patches)
	if err != nil {
		log.Fatalf("unwraping patchesStrategicMerge from %q failed error: %s", kustomizeFile, err)
	}
	for _, patchFile := range kustomizePatches {
		if !strings.HasPrefix(patchFile, "http") {
			backupFile(patchFile)
			evaluateTemplates(patchFile)
		}
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
			log.Fatalf("marshaling updated file content failed error: %s", err)
		}
		err = os.WriteFile(kustomizeFile, updatedFileContent, os.ModePerm)
		if err != nil {
			log.Fatalf("writing updated kustomize file %q failed error: %s", kustomizeFile, err)
		}
	}
}

func backupFile(kustomizeFile string) {
	dst, err := os.Create(fmt.Sprintf("%s_%s%s", kustomizeFile, time.Now().Format("20060102-150405"), filepath.Ext(kustomizeFile)))
	defer dst.Close()
	src, err := os.Open(kustomizeFile)
	defer src.Close()
	if err == nil {
		_, err := io.Copy(dst, src)
		if err != nil {
			log.Printf("backup of kustomize file %s failed. skipping backup!", kustomizeFile)
		}
	}
}

func evaluateTemplates(kustomizeFile string) {
	// evaluate templates
	fileContent, err := os.ReadFile(kustomizeFile)
	if err != nil {
		log.Fatalf("reading kustomization file %q failed error: %s", kustomizeFile, err)
	}
	evaluated, err := template.EvalGoTemplates(string(fileContent))
	if err != nil {
		log.Fatalf("template evaluation of result failed error: %s", err)
	}
	err = os.WriteFile(kustomizeFile, []byte(evaluated), os.ModePerm)
	if err != nil {
		log.Fatalf("writing evaluated kustomize file %q failed error: %s", kustomizeFile, err)
	}
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
