package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/zhouyangchao/LinkTreeScraper/linktree"
)

func main() {
	if len(os.Args) < 2 {
		fmt.Println("Username or URL is needed!")
		os.Exit(1)
	}

	input := os.Args[1]
	var username, url string

	if strings.Contains(input, "linktr.ee") {
		url = input
	} else {
		username = input
	}

	lt, err := linktree.NewLinktree("")
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}
	userInfo, jsonInfo, err := lt.GetLinktreeUserInfo(url, username)
	if err != nil {
		fmt.Printf("Error: %v, %v\n", err, jsonInfo)
		os.Exit(1)
	}

	fmt.Printf("username: %s\n", userInfo.Username)
	fmt.Printf("avatar image: %s\n", userInfo.AvatarImage)
	fmt.Printf("tier: %s\n", userInfo.Tier)
	fmt.Printf("isActive: %v\n", userInfo.IsActive)
	fmt.Printf("description: %s\n", userInfo.Description)
	fmt.Printf("createdAt: %d\n", userInfo.CreatedAt)
	fmt.Printf("updatedAt: %d\n", userInfo.UpdatedAt)

	fmt.Println("\nLinks:")
	for _, link := range userInfo.Links {
		fmt.Println(link.URL)
	}
}
