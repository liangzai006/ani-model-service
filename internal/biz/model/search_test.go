package model

import "testing"

func TestSearchRequestValidatesProviderAndKeyword(t *testing.T) {
	if err := (SearchRequest{Source: "modelscope", Keyword: "qwen", Limit: 20}).Validate(); err != nil {
		t.Fatal(err)
	}
	if err := (SearchRequest{Source: "unknown", Keyword: "qwen", Limit: 20}).Validate(); err == nil {
		t.Fatal("unknown source accepted")
	}
	if err := (SearchRequest{Source: "modelscope", Keyword: "  ", Limit: 20}).Validate(); err == nil {
		t.Fatal("blank keyword accepted")
	}
}
