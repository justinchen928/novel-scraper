package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"strings"

	"github.com/bmaupin/go-epub"
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

func (n Novel) toEpub() (*epub.Epub, error) {

	err := os.Remove((n.Name + ".epub"))
	if err != nil {
		fmt.Println(err)
	}

	e := epub.NewEpub(n.Name)

	e.SetAuthor(n.Author)
	log.Println(n.Name, n.Author)

	coverCSSPath, _ := e.AddCSS("cover.css", "")
	coverImagePath, _ := e.AddImage(n.Cover, n.Cover)
	e.SetCover(coverImagePath, coverCSSPath)

	_, err = e.AddFont("redacted-script-regular.ttf", "font.ttf")
	if err != nil {
		log.Fatal(err)
	}

	var desc string
	for _, description := range n.Description {
		desc += "<p>" + description + "</p>\n"
	}
	e.SetDescription(desc)

	for _, chapter := range n.Chapters {

		sectionBody := "<h1>" + chapter.Title + "</h1>\n<p></p>\n"

		for _, paragraph := range chapter.paragraph {
			sectionBody += "<p>" + paragraph + "</p>\n"
		}

		if _, err := e.AddSection(sectionBody, chapter.Title, "", ""); err != nil {
			return nil, err
		}
	}

	return e, nil
}

func parseLine(line string, n *Novel, count int) (c int) {
	if len(line) == 0 {
		return count
	}

	runeLine := []rune(line)

	// fmt.Printf("%s \n", string(runeLine))
	switch runeLine[0] {
	case '*': // novel header
		hasRightAngleQuotationMark := false
		var title []rune
		i := 1

		for ; i < len(runeLine); i++ {
			if runeLine[i] == '*' {
				hasRightAngleQuotationMark = true
				break
			}
			title = append(title, runeLine[i])
		}
		if !hasRightAngleQuotationMark {
			log.Println("novel title doesn't have matched angle quotation marks")
			return count
		}

		n.Name = string(title)
	case '<', '(': // parse title of paragraph
		chapter := Chapter{}
		chapter.Title = string(runeLine[1 : len(runeLine)-1])
		// log.Printf("%s", chapter.Title)
		count += 1
		n.Chapters = append(n.Chapters, chapter)
	case '#': // parse content of paragraph
		// log.Printf("%s", string(runeLine[1:]))
		n.Chapters[count-1].paragraph = append(n.Chapters[count-1].paragraph, string(runeLine[1:]))
	default:
		return count
	}

	return count
}

func safeClose(closer io.Closer) {
	if err := closer.Close(); err != nil {
		log.Panicf("error: %v", err)
	}
}

func writeText(novel Novel) {
	err := os.Remove((novel.Name + ".txt"))
	if err != nil {
		fmt.Println(err)
		// return
	}
	f, err := os.OpenFile((novel.Name + ".txt"), os.O_WRONLY|os.O_CREATE, 0600)
	if err != nil {
		panic(err)
	}

	var text = "*" + novel.Name + "*\n\n"
	for _, chapter := range novel.Chapters {
		text += chapter.Title
		text += "\n"
		for _, paragraph := range chapter.paragraph {
			text += paragraph
		}
	}

	// fmt.Println("Visiting", text)

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
		// fmt.Println("Visiting", request.URL.String())
		chapter = Chapter{}
	})

	collector.OnHTML(".info > a", func(element *colly.HTMLElement) {
		novel.Name = element.Text
		// fmt.Println("Name", element.Text)
	})

	collector.OnHTML(".content > h1", func(element *colly.HTMLElement) {
		if regexp.MustCompile("番外.*章").MatchString(element.Text) {
			chapter.Title = "(" + element.Text + ")"
		} else {
			chapter.Title = "<" + element.Text + ">"
		}
		// fmt.Println("Title", element.Text)
	})

	collector.OnHTML(".content > div.neirong", func(element *colly.HTMLElement) {
		element.ForEach("p", func(_ int, p_element *colly.HTMLElement) {
			chapter.paragraph = append(chapter.paragraph, "#")
			paragraph := strings.TrimSpace(p_element.Text)
			reg := regexp.MustCompile(`(<|{|<| )`)
			paragraph = reg.ReplaceAllString(paragraph, "")
			chapter.paragraph = append(chapter.paragraph, paragraph)
			chapter.paragraph = append(chapter.paragraph, "\n\n")
		})
		// fmt.Println("Content", chapter.Content)
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

func toEpub(novel *Novel) {
	file, err := os.Open(os.Args[1])
	if err != nil {
		log.Panicf("error: %v", err)
	}
	defer safeClose(file)

	// novel := Novel{}
	scanner := bufio.NewScanner(file)
	// lineId := uint64(1)

	log.Printf("processing %v...", os.Args[1])
	log.Println("parsing...")

	// lastStat := false
	count := 0
	for scanner.Scan() {
		line := scanner.Text()
		// log.Printf("%s", line)
		count = parseLine(line, novel, count)
		// count = parseLine(line, &novel, count)
	}

	log.Println("parse: done")

	log.Println("converting to epub...")

	e, err := novel.toEpub()

	if err != nil {
		log.Panicf("error: %v", err)
	}

	log.Println("convert: done")

	if novel.Name == "" {
		log.Println("novel doesn't have a Name, use filename instead")
		novel.Name = os.Args[1]
	}

	log.Printf("writing %s to disk...", novel.Name+".epub")

	if err = e.Write(novel.Name + ".epub"); err != nil {
		log.Panicf("error: %v", err)
	}

	log.Println("write: done")
	log.Println("process: done")
}

func downloadFile(URL, fileName string) error {
	//Get the response bytes from the url
	response, err := http.Get(URL)
	if err != nil {
		return err
	}
	defer response.Body.Close()

	if response.StatusCode != 200 {
		return errors.New("received non 200 response code")
	}
	//Create a empty file
	file, err := os.Create(fileName)
	if err != nil {
		return err
	}
	defer file.Close()

	//Write the bytes to the fiel
	_, err = io.Copy(file, response.Body)
	if err != nil {
		return err
	}

	return nil
}

func getImageAndAuthor(index_link string, novel *Novel) {
	domain := "www.hetubook.com"
	collector := colly.NewCollector(
		colly.Async(true),
		colly.AllowedDomains(domain),
	)

	collector.OnRequest(func(request *colly.Request) {
		fmt.Println("Visiting", request.URL.String())
	})

	collector.Limit(&colly.LimitRule{
		DomainGlob:  "*",
		Parallelism: 1,
	})

	collector.OnHTML("div.book_info", func(element *colly.HTMLElement) {
		element.ForEach("div > p", func(_ int, p_element *colly.HTMLElement) {
			novel.Description = append(novel.Description, strings.TrimSpace(p_element.Text))
		})
		var author []string
		element.ForEach("div>a", func(_ int, a_element *colly.HTMLElement) {
			author = append(author, strings.TrimSpace(a_element.Text))
		})
		url := element.Request.AbsoluteURL(element.ChildAttr("img", "src"))
		novel.Name = element.ChildText("h2")
		novel.Author = author[0]
		novel.Cover = novel.Name + ".jpg"
		log.Println(novel.Name, novel.Author, novel.Cover, url)
		downloadFile("https://www.gpdf.net/wp-content/uploads/2021/01/s27142913.jpg", novel.Cover)
	})

	collector.Visit(index_link)
	collector.Wait()
}

func main() {
	novel := Novel{}
	getImageAndAuthor("https://www.hetubook.com/book/2060/index.html", &novel)
	shuchengCrawler("https://www.51shucheng.net/zh-tw/wangluo/xuezhonghandaoxing/50544.html")
	toEpub(&novel)
}
