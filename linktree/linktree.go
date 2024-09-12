package linktree

import (
	"encoding/json"
	"fmt"
	"strconv"
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
		if err := json.Unmarshal([]byte(jsonData), &data); err != nil {
			return
		}
		props, ok := data["props"].(map[string]interface{})
		if !ok {
			return
		}
		pageProps, ok := props["pageProps"].(map[string]interface{})
		if !ok {
			return
		}
		userData = pageProps
	})

	if userData == nil {
		return nil, fmt.Errorf("failed to extract user data from HTML")
	}

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
	linksData, ok := resp["links"].([]interface{})
	if !ok {
		return links, nil // 返回空链接列表而不是错误
	}

	for _, l := range linksData {
		link, ok := l.(map[string]interface{})
		if !ok {
			continue // 跳过无效的链接数据
		}
		url, ok := link["url"].(string)
		if !ok {
			continue // 跳过没有有效URL的链接
		}
		links = append(links, Link{URL: url})
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

	account, ok := data["account"].(map[string]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid account data structure")
	}

	userIDFloat, ok := account["id"].(float64)
	if !ok {
		return nil, fmt.Errorf("invalid user ID")
	}
	userID := int(userIDFloat)

	links, ok := data["links"].([]interface{})
	if !ok {
		return nil, fmt.Errorf("invalid links data structure")
	}

	var resultLinks []Link
	var censoredLinkIDs []int

	for _, l := range links {
		link := l.(map[string]interface{})
		var id int
		switch v := link["id"].(type) {
		case float64:
			id = int(v)
		case string:
			var err error
			id, err = strconv.Atoi(v)
			if err != nil {
				return nil, fmt.Errorf("failed to convert id to int: %v", err)
			}
		default:
			return nil, fmt.Errorf("unexpected type for id: %T", v)
		}

		url, urlExists := link["url"].(string)
		locked, _ := link["locked"].(bool)

		if linkType, ok := link["type"].(string); ok && linkType == "COMMERCE_PAY" {
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

func (lt *Linktree) GetLinktreeUserInfo(url, username string) (*LinktreeUser, map[string]interface{}, error) {
	if url == "" && username == "" {
		return nil, nil, fmt.Errorf("Please pass linktree username or url")
	}

	jsonInfo, err := lt.getUserInfoJSON("", url, username)
	if err != nil {
		return nil, nil, err
	}

	account, ok := jsonInfo["account"].(map[string]interface{})
	if !ok {
		return nil, jsonInfo, fmt.Errorf("invalid account data structure")
	}

	username, ok = account["username"].(string)
	if !ok {
		return nil, jsonInfo, fmt.Errorf("invalid username")
	}

	avatarImage := ""
	if profilePic, ok := account["profilePictureUrl"].(string); ok {
		avatarImage = profilePic
	}

	if url == "" {
		url = fmt.Sprintf("https://linktr.ee/%s", username)
	}

	var id int
	switch v := account["id"].(type) {
	case float64:
		id = int(v)
	case string:
		var err error
		id, err = strconv.Atoi(v)
		if err != nil {
			id = 0 // Use default value instead of returning an error
		}
	default:
		id = 0 // Use default value for unexpected types
	}

	tier, _ := account["tier"].(string)
	if tier == "" {
		tier = "Unknown"
	}

	isActive := false
	if active, ok := account["isActive"].(bool); ok {
		isActive = active
	}

	var createdAt, updatedAt int64
	if ca, ok := account["createdAt"].(float64); ok {
		createdAt = int64(ca)
	}
	if ua, ok := account["updatedAt"].(float64); ok {
		updatedAt = int64(ua)
	}

	description := ""
	if desc, ok := account["description"].(string); ok {
		description = desc
	}

	links, err := lt.getUserLinks("", jsonInfo)
	if err != nil {
		fmt.Printf("Error getting user links: %v\n", err)
		links = []Link{}
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
	}, jsonInfo, nil
}
