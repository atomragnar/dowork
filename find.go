package dowork

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/charmbracelet/log"
)

type Result interface {
	unwrap() (Result, error)
}

type SearchWorkList struct {
	WorkList
	Results *chan Result
}

type FileContentResult struct {
	Path string
	Line int
	Text string
}

func (r *FileContentResult) unwrap() (Result, error) {
	return r, nil
}

func NewFilecontentResult(text string, line int, path string) *FileContentResult {
	return &FileContentResult{Text: text, Line: line, Path: path}
}

type SearchFileContentJob struct {
	path    string
	find    string
	results chan<- Result
}

func newFileContentSearch(path, find string, results chan<- Result) *SearchFileContentJob {
	return &SearchFileContentJob{path: path, find: find, results: results}
}

// Do method for InFileJob to perform the search
func (job *SearchFileContentJob) Do() error {
	file, err := os.Open(job.path)
	if err != nil {
		log.Printf("Error opening file %s: %v\n", job.path, err)
		return err
	}
	defer func() {
		if r := recover(); r != nil {
			log.Printf("Recovered in Do: %v", r)
		}
	}()

	defer func(file *os.File) {
		closErr := file.Close()
		if closErr != nil {
			log.Errorf("Error closing file %s: %v\n", job.Path, closErr)
		}
	}(file)

	scanner := bufio.NewScanner(file)
	lineNum := 1
	for scanner.Scan() {
		if strings.Contains(scanner.Text(), job.find) {
			result := NewFilecontentResult(scanner.Text(), lineNum, job.Path)
			job.results <- result
		}
		lineNum++
	}
	return nil
}

// func numberOfFilesInDir(path string) int {
// 	entries, err := os.ReadDir(path)
// 	if err != nil {
// 		fmt.Println("ReadDir error:", err)
// 		return 0
// 	}
// 	return len(entries)
// }

func numberOfFilesInDirRecursive(path string) int {
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Println("ReadDir error:", err)
		return 0
	}
	count := 0
	for _, entry := range entries {
		if entry.IsDir() {
			count += numberOfFilesInDirRecursive(filepath.Join(path, entry.Name()))
		} else {
			count++
		}
	}
	return count
}

func addFileEntryToJob(entry os.DirEntry, wl *WorkList, results chan<- Result, path, find string) {
	path = filepath.Join(path, entry.Name())
	if entry.IsDir() {
		discoverFileContentSearchDirs(wl, path, find, results)
	} else {
		job := newFileContentSearch(path, find, results)
		wl.Add(job)
	}
}

func discoverFileContentSearchDirs(wl *WorkList, path string, find string, results chan<- Result) {
	entries, err := os.ReadDir(path)
	if err != nil {
		fmt.Println("ReadDir error:", err)
		return
	}
	for _, entry := range entries {
		addFileEntryToJob(entry, wl, results, path, find)
	}
}

func SearchFileContent(searchTerm, searchDir string) <-chan Result {
	var wg sync.WaitGroup

	numWorkers := numberOfFilesInDirRecursive(searchDir)

	log.Info("Number of files to search:", numWorkers)

	results := make(chan Result, numWorkers) // Adjust buffer size as necessary
	wl := NewWorkList(numWorkers)

	workerFunc := func() {
		defer wg.Done()
		for job := range wl.Jobs() { // Assuming Jobs() provides a channel of Job interface
			err := job.Do()
			var jobsDoneErr *WorkIsDoneError
			if errors.As(err, &jobsDoneErr) {
				return // work is done
			} else if err != nil {
				log.Errorf("Error processing job: %v", err)
			}
		}
	}

	wg.Add(numWorkers + 1)

	go func() {
		defer wg.Done()
		discoverFileContentSearchDirs(wl, searchDir, searchTerm, results)
		wl.Finalize(numWorkers)
	}()

	for i := 0; i < numWorkers; i++ {
		go workerFunc()
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	return results
}
