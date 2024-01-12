package internal

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"golang.org/x/exp/slog"
)

type ResultCallBack func(data *string, err error)

type Job struct {
	data     *string
	callback *func(data *string, err error)
	done     *chan bool
}

type WorkerData struct {
	cmd       *exec.Cmd
	in        *io.WriteCloser
	out       *io.ReadCloser
	err       *io.ReadCloser
	outReader *bufio.Reader
	errReader *bufio.Reader
}

func RunAsServer() {
	workersNumber, err := strconv.Atoi(os.Getenv("NUMBER_OF_WORKERS"))
	if err != nil {
		workersNumber = 1
	}

	slog.Info(fmt.Sprintf("Starting the service with %d workers", workersNumber))

	jobs := make(chan Job, 20)
	var wg sync.WaitGroup

	for i := 0; i < workersNumber; i++ {
		go worker(jobs, &wg)
	}

	http.HandleFunc("/api/v1/dot", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		b, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "Unable to load request body.", http.StatusBadRequest)
			return
		}
		defer r.Body.Close()

		done := make(chan bool)
		ab := func(data *string, err error) {
			if err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}

			if strings.TrimSpace(*data) == "" {
				http.Error(w, "An unknown error occurred while processing dot", http.StatusInternalServerError)
				return
			}

			fmt.Fprint(w, *data)
		}

		data := string(b)

		jobs <- Job{
			data:     &data,
			callback: &ab,
			done:     &done,
		}

		<-done
		close(done)
	})

	server := &http.Server{}

	go func() {
		port, err := strconv.Atoi(os.Getenv("PORT"))
		if err != nil {
			port = 8080
		}

		listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
		if err != nil {
			slog.Error(fmt.Sprintf("Error listening on the port: %v", err))
		}

		slog.Info(fmt.Sprintf("HTTP server started on port: %d", port))
		if err := server.Serve(listener); !errors.Is(err, http.ErrServerClosed) {
			fmt.Printf("\033[1A\033[K")
			slog.Error(fmt.Sprintf("Error starting HTTP server: %v", err))
		}
		slog.Info("Stopped serving new connections.")
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	<-sigChan

	shutdownCtx, shutdownRelease := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownRelease()

	if err := server.Shutdown(shutdownCtx); err != nil {
		slog.Error("HTTP shutdown error: %v", err)
	}

	close(jobs)
	wg.Wait()

	slog.Info("Graceful shutdown complete.")
}

func getMemoryUsage(pid int) (int, error) {
	filePath := fmt.Sprintf("/proc/%d/statm", pid)

	content, err := os.ReadFile(filePath)
	if err != nil {
		return 0, err
	}

	contentStr := string(content)

	fields := strings.Fields(contentStr)

	if len(fields) > 0 {
		totalMemory, err := strconv.Atoi(fields[1])
		if err != nil {
			return 0, err
		}

		return totalMemory * os.Getpagesize(), nil
	}

	return 0, fmt.Errorf("Unable to parse memory usage for PID %d", pid)
}

func worker(jobs chan Job, wg *sync.WaitGroup) {
	var worker WorkerData
	startNewWorkerProcess(&worker)

	slog.Info(fmt.Sprintf("Worker process started with PID %d", worker.cmd.Process.Pid))

	wg.Add(1)
	defer wg.Done()

	for job := range jobs {
		slog.Debug(fmt.Sprintf("DOT directed to the worker %d: ", worker.cmd.Process.Pid))

		fmt.Fprint(*worker.in, *job.data, "\x04")

		errText, err := worker.errReader.ReadString('\x04')
		if err != nil {
			errText = ""
		}

		outText, _ := worker.outReader.ReadString('\x04')
		if err != nil {
			outText = ""
		}

		err = nil
		errText = strings.TrimSuffix(errText, "\x04")
		if errText != "" {
			err = errors.New(errText)
		}

		outText = strings.TrimSuffix(outText, "\x04")
		(*job.callback)(&outText, err)

		*job.done <- true

		memory, _ := getMemoryUsage(worker.cmd.Process.Pid)
		if memory > 1024*1024*50 {
			lastPID := worker.cmd.Process.Pid
			startNewWorkerProcess(&worker)
			slog.Info(fmt.Sprintf("Worker %d was replaced by %d due to excessive memory usage", lastPID, worker.cmd.Process.Pid))
		}
	}

	worker.cmd.Process.Signal(syscall.SIGTERM)
	slog.Info(fmt.Sprintf("Closing signal sent to worker %d", worker.cmd.Process.Pid))

	worker.cmd.Wait()
}

func startNewWorkerProcess(workerData *WorkerData) {

	if workerData.cmd != nil {
		(*workerData.in).Close()
		(*workerData.out).Close()
		(*workerData.err).Close()

		workerData.cmd.Process.Signal(syscall.SIGTERM)
		workerData.cmd.Wait()
	}

	cmd := exec.Command(os.Args[0], "worker")

	in, _ := cmd.StdinPipe()
	out, _ := cmd.StdoutPipe()
	err, _ := cmd.StderrPipe()

	workerData.cmd = cmd
	workerData.in = &in
	workerData.out = &out
	workerData.err = &err
	workerData.outReader = bufio.NewReader(out)
	workerData.errReader = bufio.NewReader(err)

	cmd.Start()
}
