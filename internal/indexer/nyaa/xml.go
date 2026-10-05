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

// NyaaItem maps an item in the Nyaa/Sukebei RSS feed. Namespaces are left unset on purpose: the two sites declare different ones.
type NyaaItem struct {
	Title      string `xml:"title"`
	Link       string `xml:"link"`
	GUID       string `xml:"guid"`
	PubDate    string `xml:"pubDate"`
	Seeders    int    `xml:"seeders"`
	Leechers   int    `xml:"leechers"`
	Downloads  int    `xml:"downloads"`
	InfoHash   string `xml:"infoHash"`
	CategoryId string `xml:"categoryId"`
	Category   string `xml:"category"`
	Size       string `xml:"size"`
	Comments   int    `xml:"comments"`
	IsTrusted  string `xml:"trusted"`
	IsRemake   string `xml:"remake"`
}
