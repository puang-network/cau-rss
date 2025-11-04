package main

import (
    "bufio"
    "fmt"
    "html/template"
    "net/url"
    "os"
    "path"
    "sort"
    "time"

    "github.com/gorilla/feeds"
    cp "github.com/otiai10/copy"
    "litehell.info/cau-rss/cau_parser"
    "litehell.info/cau-rss/server"
)

type CrawlSuccessItem struct {
    Articles  []cau_parser.CAUArticle `json:"articles"`
    SiteInfo  server.CauWebsite       `json:"site_info"`
    Timestamp int64                   `json:"timestamp"`
}

type CrwalFailureItem struct {
    SiteInfo  server.CauWebsite
    Timestamp int64 `json:"timestamp"`
}

type FeedDataResponse struct {
    Success []CrawlSuccessItem `json:"success"`
    Failure []CrwalFailureItem `json:"failure"`
}

func getAllSeoulDormitoryArticles(siteInfo *server.CauWebsite, items *[]CrawlSuccessItem) []cau_parser.CAUArticle {
    articles := make([]cau_parser.CAUArticle, 0)

    for _, item := range *items {
        if item.SiteInfo.Key == "dormitory/seoul/bluemir" ||
            item.SiteInfo.Key == "dormitory/seoul/future_house" ||
            item.SiteInfo.Key == "dormitory/seoul/global_house" {
            articles = append(articles, item.Articles...)
        }
    }

    sort.Slice(articles, func(a, b int) bool {
        return articles[a].Date.Before(articles[b].Date)
    })

    return articles
}

func generateIndex(dir string) {
    indexTemplate, err := template.New("").Funcs(
        template.FuncMap{
            "encodeURI": func(uri string) string {
                return url.QueryEscape(uri)
            },
        },
    ).ParseFiles("html/index.html")
    if err != nil {
        panic(err)
    }

    indexOutputFile, err := os.Create(path.Join(dir, "index.html"))
    if err != nil {
        panic(err)
    }

    writer := bufio.NewWriter(indexOutputFile)
    indexTemplate.ExecuteTemplate(writer, "index.html", map[string]any{
        "table":      server.GetFeedHtmlTable(),
        "webAddress": "rss.puang.network",
    })

    defer func() {
        writer.Flush()
        indexOutputFile.Close()
    }()
}

func generateFeedFiles(dir string, items *FeedDataResponse) {
    for _, item := range items.Success {
        feedName := item.SiteInfo.Name
        if item.SiteInfo.LongName != "" {
            feedName = item.SiteInfo.LongName
        }
        feed := &feeds.Feed{
            Title:       fmt.Sprintf("%s 공지사항", feedName),
            Link:        &feeds.Link{Href: item.SiteInfo.Url},
            Description: fmt.Sprintf("%s의 공지사항입니다", feedName),
        }

        rss, err := server.GenerateFeed(feed, item.Articles, server.RSS)
        if err != nil {
            panic(err)
        }

        atom, err := server.GenerateFeed(feed, item.Articles, server.ATOM)
        if err != nil {
            panic(err)
        }

        json, err := server.GenerateFeed(feed, item.Articles, server.JSON)
        if err != nil {
            panic(err)
        }

        feedDir := path.Join(dir, "cau", item.SiteInfo.Key)
        err = os.MkdirAll(feedDir, 0755)
        if err != nil {
            panic(err)
        }

        err = os.WriteFile(path.Join(feedDir, "rss"), []byte(rss), 0644)
        if err != nil {
            panic(err)
        }

        err = os.WriteFile(path.Join(feedDir, "atom"), []byte(atom), 0644)
        if err != nil {
            panic(err)
        }

        err = os.WriteFile(path.Join(feedDir, "json"), []byte(json), 0644)
        if err != nil {
            panic(err)
        }

    }
}

func generateStatic(dir string, items *FeedDataResponse) {
    generateIndex(dir)
    generateFeedFiles(dir, items)
    cp.Copy("static", dir)
}

func main() {
    fmt.Println("Starting static site generation for Cloudflare Pages...")

    success := make([]CrawlSuccessItem, 0)
    failures := make([]CrwalFailureItem, 0)
    var allSeoulDormitory server.CauWebsite

    server.LoopForAllSites(func(site *server.CauWebsite) {
        if site.Key == "dormitory/seoul/all" {
            allSeoulDormitory = *site
            return
        }
        articles, articlesErr := server.FetchArticlesForKey(site.Key)
        if articlesErr != nil {
            failures = append(failures, CrwalFailureItem{
                SiteInfo:  *site,
                Timestamp: time.Now().Unix(),
            })
            return
        } else {
            success = append(success, CrawlSuccessItem{
                SiteInfo:  *site,
                Articles:  articles,
                Timestamp: time.Now().Unix(),
            })
        }
    })

    success = append(success, CrawlSuccessItem{
        SiteInfo:  allSeoulDormitory,
        Articles:  getAllSeoulDormitoryArticles(&allSeoulDormitory, &success),
        Timestamp: time.Now().Unix(),
    })

    res := FeedDataResponse{
        Success: success,
        Failure: failures,
    }

    outDir := "public"
    // ensure fresh
    os.RemoveAll(outDir)
    if err := os.MkdirAll(outDir, 0755); err != nil {
        panic(err)
    }

    generateStatic(outDir, &res)

    fmt.Printf("Static site generated to %s (entries: %d, failures: %d)\n", outDir, len(res.Success), len(res.Failure))
}
