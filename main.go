// main.go
package main

import (
    "fmt"
    "net/http"
	"time"
	"os"
    "io/ioutil"
    "github.com/gocolly/colly/v2"
)

var payingQueue chan string
var nonPayingQueue chan string

type CrawlRequest struct {
    URL     string
    IsPaying bool
    Writer  http.ResponseWriter
}

var crawlQueue chan CrawlRequest

func main() {
    crawlQueue = make(chan CrawlRequest, 100) // Adjust the queue size as needed

    // Start the crawlers
    go startCrawler(true)
    go startCrawler(false)

    // Create a new HTTP server
    http.HandleFunc("/", indexHandler)
    http.HandleFunc("/crawl", crawlHandler)
    http.ListenAndServe(":8080", nil)
}

func indexHandler(w http.ResponseWriter, r *http.Request) {
    http.ServeFile(w, r, "index.html")
}


// func crawlHandler(w http.ResponseWriter, r *http.Request) {
//     isPaying := r.URL.Query().Get("paying") == "true"
//     url := r.URL.Query().Get("url")

//     if isCrawledRecently(url) {
//         if isPaying {
//             payingQueue <- url // Prioritize paying customers
//         } else {
//             nonPayingQueue <- url
//         }
//     } else {
//         fmt.Fprint(w, "Error: Webpage not crawled or too old")
//     }
// }

// func crawlHandler(w http.ResponseWriter, r *http.Request) {
//     isPaying := r.URL.Query().Get("paying") == "true"
//     url := r.URL.Query().Get("url")

//     if isCrawledRecently(url) {
//         if isPaying {
//             // Check for cached data for paying customers
//             cachedPage, err := readCrawledPageFromDisk(url)
//             if err == nil {
//                 fmt.Fprint(w, cachedPage)
//                 return
//             }
//         } else {
//             // Check for cached data for non-paying customers
//             cachedPage, err := readCrawledPageFromDisk(url)
//             if err == nil {
//                 fmt.Fprint(w, cachedPage)
//                 return
//             }
//         }
//     } else {
//         // If no cached data is available, proceed to crawl the page
//         fmt.Fprint(w, "Crawling in real-time...")
//         if isPaying {
//             payingQueue <- url // Prioritize paying customers
//         } else {
//             nonPayingQueue <- url
//         }
//     }
// }

func crawlHandler(w http.ResponseWriter, r *http.Request) {
    isPaying := r.URL.Query().Get("paying") == "true"
    url := r.URL.Query().Get("url")

    if isCrawledRecently(url) {
        crawlQueue <- CrawlRequest{URL: url, IsPaying: isPaying, Writer: w}
    } else {
        // If the URL is not crawled recently, attempt to serve cached data
        cachedPage, err := readCrawledPageFromDisk(url)
        if err == nil {
            fmt.Fprint(w, cachedPage)
            return
        }

        // If no cached data is available, proceed to crawl the page
        fmt.Fprint(w, "Crawling in real-time...")
        crawlQueue <- CrawlRequest{URL: url, IsPaying: isPaying, Writer: w}
    }
}


// func startCrawler(isPaying bool, queue chan string) {
//     for {
//         select {
//         case url := <-queue:
//             webpages, err := crawlPage(url, isPaying)
//             if err != nil {
//                 fmt.Printf("Error crawling %s: %v\n", url, err)
//             } else {
//                 for _, pg := range webpages {
//                     // Store or process the webpages as needed
//                     fmt.FPrintln("Crawled %s: %s\n", url, pg)
//                 }
//             }
//         }
//     }
// }

func startCrawler(isPaying bool) {
    for {
        request := <-crawlQueue
        if request.IsPaying == isPaying {
            webpages, err := crawlPage(request.URL, isPaying)
            if err != nil {
                fmt.Printf("Error crawling %s: %v\n", request.URL, err)
                fmt.Fprint(request.Writer, "Error: Page not found or couldn't be crawled.")
            } else {
                for _, pg := range webpages {
                    // Store or process the webpages as needed
                    fmt.Printf("Crawled %s: %s\n", request.URL, pg)
                    fmt.Fprintln(request.Writer, pg)
                }

            }
        }
    }
}


// This map stores the last crawl time for each URL.
var lastCrawlTimes map[string]time.Time

func isCrawledRecently(url string) bool {
    // Check if the URL exists in the lastCrawlTimes map.
    if lastCrawlTime, ok := lastCrawlTimes[url]; ok {
        // Calculate the time elapsed since the last crawl.
        elapsed := time.Since(lastCrawlTime)
        // If the elapsed time is less than 60 minutes, return true.
        return elapsed.Minutes() <= 60
    }
    // If the URL is not found in the map, return false.
    return false
}

// Directory where crawled pages are stored
const crawlDataDir = "crawl_data"

func readCrawledPageFromDisk(url string) (string, error) {
    // Construct the file path based on the URL (you might need to sanitize it for use as a file name)
    filePath := crawlDataDir + "/" + url + ".html"

    // Attempt to open the file
    file, err := os.Open(filePath)
    if err != nil {
        return "", err
    }
    defer file.Close()

    // Read the content from the file
    content, err := ioutil.ReadAll(file)
    if err != nil {
        return "", err
    }

    return string(content), nil
}


var (
    crawlRateLimiter = make(chan struct{}, 5)
    maxRetries = 3
)


func crawlPage(url string, isPaying bool) ([]string, error) {
    retries := 0

    var webpages []string
    var crawlError error

    for {
        select {
        case crawlRateLimiter <- struct{}{}:
            c := colly.NewCollector()

            c.OnError(func(r *colly.Response, err error) {
                retries++
                if retries <= maxRetries {
                    fmt.Printf("Failed to crawl %s, retrying (%d/%d)\n", url, retries, maxRetries)
                    time.Sleep(2 * time.Second)
                    crawlError = err
                    return
                }
            })

            c.OnHTML("a", func(e *colly.HTMLElement) {
                link := e.Attr("href")
                webpages = append(webpages, link)
            })

            err := c.Visit(url)
            if err != nil {
                crawlError = err
            }

            if crawlError != nil {
                return nil, crawlError
            }

            return webpages, nil
        }
    }
}

