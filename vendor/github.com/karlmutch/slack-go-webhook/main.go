package slack

type Field struct {
	Title string `json:"title"`
	Value string `json:"value"`
	Short bool   `json:"short"`
}

type Attachment struct {
	Fallback    *string  `json:"fallback"`
	Color       *string  `json:"color"`
	PreText     *string  `json:"pretext"`
	AuthorName  *string  `json:"author_name"`
	AuthorLink  *string  `json:"author_link"`
	AuthorIcon  *string  `json:"author_icon"`
	Title       *string  `json:"title"`
	TitleLink   *string  `json:"title_link"`
	Text        *string  `json:"text"`
	Timestamp   *int64   `json:"ts"`
	UnfurlLinks *bool    `json:"unfurl_links"`
	UnfurlMedia *bool    `json:"unfurl_media"`
	ImageUrl    *string  `json:"image_url"`
	Fields      []*Field `json:"fields"`
	Footer      *string  `json:"footer"`
	FooterIcon  *string  `json:"footer_icon"`
}

// Message is used to marshal a JSON data structure for use with the Slack
// webhook interface.  Messages use UTF-8. Be sure to make use of URL escapes for
// the ampersand (&amp;), less than (&lt;) and greater than (&gt;) signs.a Do NOT
// HTML entity escape the entire set of characters.
//
// Message formatting is documented at the following URL,
// https://api.slack.com/docs/message-formatting
//
type Message struct {
	Parse       string       `json:"parse,omitempty"`
	Username    string       `json:"username,omitempty"`
	IconUrl     string       `json:"icon_url,omitempty"`
	IconEmoji   string       `json:"icon_emoji,omitempty"`
	Channel     string       `json:"channel,omitempty"`
	Text        string       `json:"text,omitempty"`
	Attachments []Attachment `json:"attachments,omitempty"`
}
