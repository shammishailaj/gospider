package main

import (
	"bufio"
	"fmt"
	"github.com/theblackturtle/gospider/core"
	"net/url"

	"os"
	"strings"
	"sync"

	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

var commands = &cobra.Command{
	Use:   core.CLIName,
	Short: "A Simple Web Spider",
	Long:  fmt.Sprintf("A Simple Web Spider by %v", core.AUTHOR),
	Run:   run,
}

func main() {
	commands.Flags().StringP("site", "s", "", "Site to crawl")
	commands.Flags().StringP("sites", "S", "", "Site list to crawl")
	commands.Flags().StringP("proxy", "p", "", "Proxy (Ex: http://127.0.0.1:8080)")
	commands.Flags().StringP("output", "o", "", "Output folder")
	commands.Flags().StringP("user-agent", "u", "web", "User Agent to use\n\tweb: random web user-agent\n\tmobi: random mobile user-agent\n\tor you can set your special user-agent")
	commands.Flags().StringP("cookie", "", "", "Cookie to use (testA=a; testB=b)")
	commands.Flags().StringArrayP("header", "H", []string{}, "Header to use (Use multiple flag to set multiple header)")
	commands.Flags().StringP("burp", "", "", "Load headers and cookie from burp raw http request")
	commands.Flags().StringP("blacklist", "", "", "Blacklist URL Regex")

	commands.Flags().IntP("threads", "t", 1, "Number of threads (Run sites in parallel)")
	commands.Flags().IntP("concurrent", "c", 5, "The number of the maximum allowed concurrent requests of the matching domains")
	commands.Flags().IntP("depth", "d", 1, "MaxDepth limits the recursion depth of visited URLs. (Set it to 0 for infinite recursion)")
	commands.Flags().IntP("delay", "k", 0, "Delay is the duration to wait before creating a new request to the matching domains (second)")
	commands.Flags().IntP("random-delay", "K", 0, "RandomDelay is the extra randomized duration to wait added to Delay before creating a new request (second)")

	commands.Flags().BoolP("sitemap", "", false, "Try to crawl sitemap.xml")
	commands.Flags().BoolP("robots", "", true, "Try to crawl robots.txt")
	commands.Flags().BoolP("other-source", "", false, "Find url from another sources")

	commands.Flags().BoolP("debug", "", false, "Turn on debug mode")
	commands.Flags().BoolP("no-redirect", "", false, "Disable redirect")

	commands.Flags().SortFlags = false
	if err := commands.Execute(); err != nil {
		core.Logger.Error(err)
		os.Exit(1)
	}
}

func run(cmd *cobra.Command, args []string) {
	if cmd.Flags().NFlag() == 0 {
		cmd.HelpFunc()(cmd, args)
		os.Exit(1)
	}

	isDebug, _ := cmd.Flags().GetBool("debug")
	if isDebug {
		core.Logger.SetLevel(logrus.DebugLevel)
	} else {
		core.Logger.SetLevel(logrus.InfoLevel)
	}

	// Create output folder when save file option selected
	outputFolder, _ := cmd.Flags().GetString("output")
	if outputFolder != "" {
		if _, err := os.Stat(outputFolder); os.IsNotExist(err) {
			_ = os.Mkdir(outputFolder, os.ModePerm)
		}
	}

	// Parse sites input
	var siteList []string
	siteInput, _ := cmd.Flags().GetString("site")
	if siteInput != "" {
		siteList = append(siteList, siteInput)
	}
	sitesInput, _ := cmd.Flags().GetString("sites")
	if sitesInput != "" {
		sitesFile, err := os.Open(siteInput)
		if err != nil {
			core.Logger.Error(err)
			os.Exit(1)
		}
		defer sitesFile.Close()

		sc := bufio.NewScanner(sitesFile)
		for sc.Scan() {
			line := strings.TrimSpace(sc.Text())
			if err := sc.Err(); err == nil && line != "" {
				siteList = append(siteList, line)
			}
		}
	}

	// Check again to make sure at least one site in slice
	if len(siteList) == 0 {
		core.Logger.Info("No site in list. Please check your site input again")
		os.Exit(1)
	}

	threads, _ := cmd.Flags().GetInt("threads")
	sitemap, _ := cmd.Flags().GetBool("sitemap")
	robots, _ := cmd.Flags().GetBool("robots")
	otherSource, _ := cmd.Flags().GetBool("other-source")
	maxDepth, _ := cmd.Flags().GetInt("depth")

	var wg sync.WaitGroup
	inputChan := make(chan string, threads)
	for i := 0; i < threads; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for site := range inputChan {
				u, err := url.Parse(site)
				if err != nil {
					logrus.Errorf("Failed to parse: %s", site)
					continue
				}

				var siteWg sync.WaitGroup

				crawler := core.NewCrawler(site, cmd)
				site = strings.TrimSuffix(u.String(), "/")
				siteWg.Add(1)
				go func() {
					crawler.Start()
					defer siteWg.Done()
				}()

				// Brute force Sitemap/Robots path
				if sitemap {
					siteWg.Add(1)
					go core.ParseSiteMap(site, maxDepth, crawler.Output, crawler.C, &siteWg)
				}

				if robots {
					siteWg.Add(1)
					go core.ParseRobots(site, maxDepth, crawler.Output, crawler.C, &siteWg)
				}

				if otherSource {
					siteWg.Add(1)
					go func() {
						defer siteWg.Done()
						urls := core.OtherSources(core.GetDomain(site))
						for _, url := range urls {
							_ = crawler.C.Visit(url)
						}
					}()
				}

				siteWg.Wait()
			}
		}()
	}

	for _, site := range siteList {
		inputChan <- site
	}
	close(inputChan)
	wg.Wait()
	core.Logger.Info("Done!!!")
}