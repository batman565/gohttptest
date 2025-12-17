package gohttptest

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"slices"
	"sync"
	"time"
)

/*
	Метод тестирования API HTTP

site-адрес переменной, count_p-количество параллельных запросов, count_r-количество запросов
*/
func Test(site string, count_p, count_r int) {

	if site == "" || count_p == 0 || count_r == 0 {
		fmt.Println("Must be 3 values: -s, -c, -n. More --help")
		flag.PrintDefaults()
		return
	}

	if len(site) > 4 && site[:4] != "http" {
		site = "http://" + site
	}

	fmt.Printf("Starting benchmark...\n")
	fmt.Printf("URL:         %s\n", site)
	fmt.Printf("Concurrency: %d\n", count_p)
	fmt.Printf("Requests:    %d\n\n", count_r)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, os.Interrupt)
	go func() {
		<-sigChan
		fmt.Println("\n\nInterrupt received, stopping...")
		cancel()
	}()

	results := make(chan result, count_r)
	var wg sync.WaitGroup

	startTime := time.Now()

	jobs := make(chan struct{}, count_r)
	for range count_r {
		jobs <- struct{}{}
	}
	close(jobs)

	for i := range count_p {
		wg.Add(1)
		go func(workerID int) {
			defer wg.Done()

			client := &http.Client{
				Timeout: 10 * time.Second,
			}

			for range jobs {
				select {
				case <-ctx.Done():
					return
				default:
					reqStart := time.Now()

					req, err := http.NewRequestWithContext(ctx, "GET", site, nil)
					if err != nil {
						results <- result{
							StatusCode: 0,
							Duration:   time.Since(reqStart),
							Error:      err,
						}
						continue
					}

					resp, err := client.Do(req)
					duration := time.Since(reqStart)

					if err != nil {
						results <- result{
							StatusCode: 0,
							Duration:   duration,
							Error:      err,
						}
						continue
					}

					bodyBytes, _ := io.ReadAll(resp.Body)
					resp.Body.Close()

					results <- result{
						StatusCode: resp.StatusCode,
						Duration:   duration,
						Bytes:      int64(len(bodyBytes)),
						Error:      nil,
					}
				}
			}
		}(i)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	var (
		totalRequests int
		successCount  int
		failedCount   int
		totalDuration time.Duration
		minDuration   = time.Hour
		maxDuration   time.Duration
		durations     []time.Duration
		totalBytes    int64
	)

	for res := range results {
		totalRequests++
		totalDuration += res.Duration
		totalBytes += res.Bytes

		if res.Duration < minDuration {
			minDuration = res.Duration
		}
		if res.Duration > maxDuration {
			maxDuration = res.Duration
		}

		durations = append(durations, res.Duration)

		if res.Error != nil || res.StatusCode >= 400 {
			failedCount++
		} else {
			successCount++
		}
	}

	totalTestTime := time.Since(startTime)

	var p50, p90, p95, p99 time.Duration
	if len(durations) > 0 {
		slices.Sort(durations)

		p50 = durations[int(float64(len(durations))*0.50)]
		p90 = durations[int(float64(len(durations))*0.90)]
		p95 = durations[int(float64(len(durations))*0.95)]
		p99 = durations[int(float64(len(durations))*0.99)]
	}

	fmt.Println("BENCHMARK RESULTS")

	fmt.Printf("Time taken:           %v\n", totalTestTime.Round(time.Millisecond))
	fmt.Printf("Total requests:       %d\n", totalRequests)
	fmt.Printf("Successful requests:  %d\n", successCount)
	fmt.Printf("Failed requests:      %d\n", failedCount)
	fmt.Printf("Requests per second:  %.2f\n", float64(totalRequests)/totalTestTime.Seconds())

	if totalRequests > 0 {
		avgDuration := totalDuration / time.Duration(totalRequests)
		fmt.Printf("Average duration:     %v\n", avgDuration.Round(time.Microsecond))
		fmt.Printf("Min duration:         %v\n", minDuration.Round(time.Microsecond))
		fmt.Printf("Max duration:         %v\n", maxDuration.Round(time.Microsecond))
		fmt.Printf("50th percentile:      %v\n", p50.Round(time.Microsecond))
		fmt.Printf("90th percentile:      %v\n", p90.Round(time.Microsecond))
		fmt.Printf("95th percentile:      %v\n", p95.Round(time.Microsecond))
		fmt.Printf("99th percentile:      %v\n", p99.Round(time.Microsecond))

		if totalTestTime > 0 {
			fmt.Printf("Throughput:           %.2f KB/s\n",
				float64(totalBytes)/1024/totalTestTime.Seconds())
		}

		successRate := float64(successCount) / float64(totalRequests) * 100
		fmt.Printf("Success rate:         %.1f%%\n", successRate)
	}

}

type result struct {
	StatusCode int
	Duration   time.Duration
	Bytes      int64
	Error      error
}
