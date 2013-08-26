package harness

import (
	"bytes"
	"errors"
	"fmt"
	"github.com/robfig/revel"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
)

var compileMap = map[string]string{
	".coffee": ".js",
	".less":   ".css",
}

/* A PrePreProcessor is used to preprocess crosscompile
assets. Its process method accepts an io.Reader, usually the origin file,
and and io.Writer, which is usually the target file */
type PreProcessor struct {
	Cmd  string
	Args []string
}

type Asset struct {
	Filepath   string
	ResultChan chan Result
}

type Result struct {
	Filepath, Cmd string
	Error         error
}

type Worker struct {
	Files     []string
	Processor *PreProcessor
}

type Pipeline struct {
	AssetPath                    string
	LessCompiler, CoffeeCompiler *PreProcessor
}

func NewPipeline() *Pipeline {

	revelPath := func(p string) string {

		if strings.Contains(p, "/") && !path.IsAbs(p) {
			return path.Join(revel.BasePath, p)
		}
		return p
	}

	assetPath := path.Join(revel.BasePath, revel.Config.StringDefault("assets.path", "assets"))

	coffee := &PreProcessor{
		Cmd:  revelPath(revel.Config.StringDefault("assets.coffee", "coffee")),
		Args: []string{"-s", "-p"},
	}
	less := &PreProcessor{
		Cmd:  revelPath(revel.Config.StringDefault("assets.less", "lessc")),
		Args: []string{"-", fmt.Sprintf("--include-path=%s", path.Join(assetPath, "css"))},
	}

	return &Pipeline{
		AssetPath:      assetPath,
		CoffeeCompiler: coffee,
		LessCompiler:   less,
	}
}

func (p *Pipeline) Refresh() (err *revel.Error) {

	var coffeeFiles, lessFiles []string

	filepath.Walk(p.AssetPath, func(path string, info os.FileInfo, err error) error {

		if !info.IsDir() {
			ext := filepath.Ext(path)
			switch ext {
			case ".coffee":
				coffeeFiles = append(coffeeFiles, path)
			case ".less":
				// skip partials
				if !strings.HasPrefix(info.Name(), "_") {
					lessFiles = append(lessFiles, path)
				}
			}

		}
		return nil
	})
	// buffered channel of len(all files)
	results := make(chan Result, len(coffeeFiles)+len(lessFiles))

	cworker := Worker{
		Files:     coffeeFiles,
		Processor: p.CoffeeCompiler,
	}

	lworker := Worker{
		Files:     lessFiles,
		Processor: p.LessCompiler,
	}

	go run(results, cworker, lworker)

	// block untill all results return
	for i := 0; i < cap(results); i++ {
		result := <-results
		if result.Error != nil {
			revel.WARN.Printf("Unable to compile (%s) : Error(%s) : Command(%s)", result.Filepath, result.Error, result.Cmd)
		}
	}
	return
}

func run(results chan Result, workers ...Worker) {
	for _, w := range workers {
		for _, path := range w.Files {
			go func(worker Worker) {
				a := Asset{
					ResultChan: results,
					Filepath:   path,
				}
				a.Compile(worker.Processor)
			}(w)

		}
	}
}

func (a Asset) Compile(processor *PreProcessor) {
	var err error
	send := func(err error) {
		result := Result{
			Filepath: a.Filepath,
			Cmd:      processor.Cmd,
			Error:    err,
		}
		a.ResultChan <- result
	}

	dir, fname := filepath.Split(a.Filepath)
	ext := filepath.Ext(a.Filepath)
	base := strings.Replace(dir, "assets", "public", 1)
	newFilename := strings.Replace(fname, ext, compileMap[ext], 1)
	newPath := filepath.Join(base, newFilename)
	if err = os.MkdirAll(base, os.ModePerm); err != nil {
		send(err)
	}

	origin, err := os.Open(a.Filepath)
	if err != nil {
		send(err)
	}
	defer origin.Close()

	buf := bytes.NewBuffer(make([]byte, 0))
	if err = processor.Process(origin, buf); err != nil {
		send(err)
		return
	}
	if err = ioutil.WriteFile(newPath, buf.Bytes(), 0666); err != nil {
		send(err)
		return
	}
	send(err)

}

func (c PreProcessor) Process(r io.Reader, w io.Writer) error {

	// create new command with processors cmd and args
	cmd := exec.Command(c.Cmd, c.Args...)

	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return err
	}

	if err = cmd.Start(); err != nil {
		return err
	}

	// read all from reader, ...
	src, err := ioutil.ReadAll(r)
	if err != nil {
		return err
	}
	// ... & pipe into stdin
	stdin.Write(src)
	stdin.Close()

	compiled, err := ioutil.ReadAll(stdout)
	if err != nil {
		return err
	}

	cmdErr, err := ioutil.ReadAll(stderr)
	if err != nil {
		return err
	}

	err = cmd.Wait()
	// return from stderr
	if len(cmdErr) > 0 {
		return errors.New(string(cmdErr))
	}

	if err != nil {
		return err
	}

	_, err = w.Write(compiled)
	if err != nil {
		return err
	}

	return nil

}
