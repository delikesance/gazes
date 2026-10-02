package nyaa

import "encoding/xml"

// NyaaRSS maps the root RSS 2.0 document.
type NyaaRSS struct {
	XMLName xml.Name    `xml:"rss"`
	Channel NyaaChannel `xml:"channel"`
}

// NyaaChannel maps the channel element in RSS 2.0.
type NyaaChannel struct {
	Title       string     `xml:"title"`
	Description string     `xml:"description"`
	Link        string     `xml:"link"`
	Items       []NyaaItem `xml:"item"`
}

// NyaaItem maps an item in the Nyaa RSS feed with custom nyaa namespace tags.
type NyaaItem struct {
	Title      string `xml:"title"`
	Link       string `xml:"link"`
	GUID       string `xml:"guid"`
	PubDate    string `xml:"pubDate"`
	Seeders    int    `xml:"https://nyaa.si/xmlns/nyaa seeders"`
	Leechers   int    `xml:"https://nyaa.si/xmlns/nyaa leechers"`
	Downloads  int    `xml:"https://nyaa.si/xmlns/nyaa downloads"`
	InfoHash   string `xml:"https://nyaa.si/xmlns/nyaa infoHash"`
	CategoryId string `xml:"https://nyaa.si/xmlns/nyaa categoryId"`
	Category   string `xml:"https://nyaa.si/xmlns/nyaa category"`
	Size       string `xml:"https://nyaa.si/xmlns/nyaa size"`
	Comments   int    `xml:"https://nyaa.si/xmlns/nyaa comments"`
	IsTrusted  string `xml:"https://nyaa.si/xmlns/nyaa trusted"`
	IsRemake   string `xml:"https://nyaa.si/xmlns/nyaa remake"`
}
