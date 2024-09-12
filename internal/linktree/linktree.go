package linktree

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PuerkitoBio/goquery"
	"github.com/valyala/fasthttp"
)

type Link struct {
	URL string `json:"url"`
}

type LinktreeUser struct {
	Username    string `json:"username"`
	URL         string `json:"url"`
	AvatarImage string `json:"avatar_image"`
	ID          int    `json:"id"`
	Tier        string `json:"tier"`
	IsActive    bool   `json:"is_active"`
	Description string `json:"description"`
	CreatedAt   int64  `json:"created_at"`
	UpdatedAt   int64  `json:"updated_at"`
	Links       []Link `json:"links"`
}

type Linktree struct {
	client *fasthttp.Client
}

func NewLinktree() *Linktree {
	return &Linktree{
		client: &fasthttp.Client{},
	}
}

func (lt *Linktree) fetch(url string, method string, headers map[string]string, body []byte) ([]byte, error) {
	req := fasthttp.AcquireRequest()
	resp := fasthttp.AcquireResponse()
	defer fasthttp.ReleaseRequest(req)
	defer fasthttp.ReleaseResponse(resp)

	req.SetRequestURI(url)
	req.Header.SetMethod(method)

	for k, v := range headers {
		req.Header.Set(k, v)
	}

	if len(body) > 0 {
		req.SetBody(body)
	}

	err := lt.client.Do(req, resp)
	if err != nil {
		return nil, err
	}

	return resp.Body(), nil
}

func (lt *Linktree) getSource(url string) (string, error) {
	body, err := lt.fetch(url, "GET", nil, nil)
	if err != nil {
		return "", err
	}
	return string(body), nil
}

func (lt *Linktree) getUserInfoJSON(source, url, username string) (map[string]interface{}, error) {
	if url == "" && username != "" {
		url = fmt.Sprintf("https://linktr.ee/%s", username)
	}

	if source == "" && url != "" {
		var err error
		source, err = lt.getSource(url)
		if err != nil {
			return nil, err
		}
	}

	doc, err := goquery.NewDocumentFromReader(strings.NewReader(source))
	if err != nil {
		return nil, err
	}

	var userData map[string]interface{}
	doc.Find("script#__NEXT_DATA__").Each(func(i int, s *goquery.Selection) {
		jsonData := s.Text()
		var data map[string]interface{}
		json.Unmarshal([]byte(jsonData), &data)
		userData = data["props"].(map[string]interface{})["pageProps"].(map[string]interface{})
	})

	return userData, nil
}

func (lt *Linktree) uncensorLinks(accountID int, linkIDs []int) ([]Link, error) {
	headers := map[string]string{
		"origin":     "https://linktr.ee",
		"referer":    "https://linktr.ee",
		"user-agent": "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/91.0.4472.77 Safari/537.36",
	}

	data := map[string]interface{}{
		"accountId": accountID,
		"validationInput": map[string]interface{}{
			"acceptedSensitiveContent": linkIDs,
		},
		"requestSource": map[string]interface{}{
			"referrer": nil,
		},
	}

	jsonData, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}

	body, err := lt.fetch("https://linktr.ee/api/profiles/validation/gates", "POST", headers, jsonData)
	if err != nil {
		return nil, err
	}

	var resp map[string]interface{}
	err = json.Unmarshal(body, &resp)
	if err != nil {
		return nil, err
	}

	links := []Link{}
	for _, l := range resp["links"].([]interface{}) {
		link := l.(map[string]interface{})
		links = append(links, Link{URL: link["url"].(string)})
	}

	return links, nil
}

func (lt *Linktree) getUserLinks(username string, data map[string]interface{}) ([]Link, error) {
	if data == nil && username != "" {
		var err error
		data, err = lt.getUserInfoJSON("", "", username)
		if err != nil {
			return nil, err
		}
	}

	userID := int(data["account"].(map[string]interface{})["id"].(float64))
	links := data["links"].([]interface{})

	var resultLinks []Link
	var censoredLinkIDs []int

	for _, l := range links {
		link := l.(map[string]interface{})
		id := int(link["id"].(float64))
		url, urlExists := link["url"].(string)
		locked := link["locked"].(bool)

		if link["type"].(string) == "COMMERCE_PAY" {
			continue
		}

		if !urlExists && locked {
			censoredLinkIDs = append(censoredLinkIDs, id)
			continue
		}

		resultLinks = append(resultLinks, Link{URL: url})
	}

	uncensoredLinks, err := lt.uncensorLinks(userID, censoredLinkIDs)
	if err != nil {
		return nil, err
	}

	resultLinks = append(resultLinks, uncensoredLinks...)
	return resultLinks, nil
}

func (lt *Linktree) GetLinktreeUserInfo(url, username string) (*LinktreeUser, error) {
	if url == "" && username == "" {
		return nil, fmt.Errorf("Please pass linktree username or url")
	}

	jsonInfo, err := lt.getUserInfoJSON("", url, username)
	if err != nil {
		return nil, err
	}

	account := jsonInfo["account"].(map[string]interface{})
	username = account["username"].(string)
	avatarImage := account["profilePictureUrl"].(string)
	if url == "" {
		url = fmt.Sprintf("https://linktr.ee/%s", username)
	}
	id := int(account["id"].(float64))
	tier, _ := account["tier"].(string)
	if tier == "" {
		tier = "Unknown"
	}
	isActive := account["isActive"].(bool)
	createdAt := int64(account["createdAt"].(float64))
	updatedAt := int64(account["updatedAt"].(float64))
	description := account["description"].(string)

	links, err := lt.getUserLinks("", jsonInfo)
	if err != nil {
		return nil, err
	}

	return &LinktreeUser{
		Username:    username,
		URL:         url,
		AvatarImage: avatarImage,
		ID:          id,
		Tier:        tier,
		IsActive:    isActive,
		CreatedAt:   createdAt,
		UpdatedAt:   updatedAt,
		Description: description,
		Links:       links,
	}, nil
}
