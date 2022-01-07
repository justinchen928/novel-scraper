package main

import (
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/gocolly/colly"
)

type Chapter struct {
	Title     string
	paragraph []string
}

type Novel struct {
	Name        string
	Author      string
	Chapters    []Chapter
	Description []string
	Cover       string
}

func writeText(novel Novel) {
	err := os.Remove((novel.Name + ".txt"))
	if err != nil {
		fmt.Println(err)
	}
	f, err := os.OpenFile((novel.Name + ".txt"), os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	var text = ""
	for _, chapter := range novel.Chapters {
		text += chapter.Title
		text += "\n"
		for _, paragraph := range chapter.paragraph {
			text += paragraph
		}
	}

	defer f.Close()

	if _, err = f.WriteString(text); err != nil {
		panic(err)
	}
}

func shuchengCrawler(first_page_link string) {
	domain := "www.51shucheng.net"
	novel := Novel{}
	novel.Chapters = make([]Chapter, 0)
	chapter := Chapter{}

	collector := colly.NewCollector(
		colly.Async(true),
		colly.AllowedDomains(domain),
	)

	collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 1,
	})

	collector.OnRequest(func(request *colly.Request) {
		chapter = Chapter{}
	})

	collector.OnHTML(".info > a", func(element *colly.HTMLElement) {
		novel.Name = element.Text
	})

	collector.OnHTML(".content > h1", func(element *colly.HTMLElement) {
		chapter.Title = element.Text
	})

	collector.OnHTML(".content > div.neirong", func(element *colly.HTMLElement) {
		element.ForEach("p", func(_ int, p_element *colly.HTMLElement) {
			paragraph := strings.TrimSpace(p_element.Text)
			reg := regexp.MustCompile(`(<|{|<| )`)
			paragraph = reg.ReplaceAllString(paragraph, "")
			chapter.paragraph = append(chapter.paragraph, paragraph)
			chapter.paragraph = append(chapter.paragraph, "\n\n")
		})
	})

	collector.OnHTML("#BookNext", func(element *colly.HTMLElement) {
		next_page_link := strings.TrimSpace(element.Attr("href"))
		r, _ := regexp.Compile(`(http|ftp|https):\\/\\/([\\w_-]+(?:(?:\\.[\\w_-]+)+))([\\w.,@?^=%&:\\/~+#-]*[\\w@?^=%&\\/~+#-])\\.html$`)
		log.Println("Link", chapter.Title, next_page_link, r.MatchString(next_page_link))
		if r.MatchString(next_page_link) {
			novel.Chapters = append(novel.Chapters, chapter)
			element.Request.Visit(next_page_link)
		} else {
			novel.Chapters = append(novel.Chapters, chapter)
			writeText(novel)
			log.Println("end")
		}
	})
	collector.Visit(first_page_link)
	collector.Wait()
}

func main() {
	shuchengCrawler("https://www.51shucheng.net/zh-tw/wangluo/xuezhonghandaoxing/50544.html")
}
