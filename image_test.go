package claudeagent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"testing"
)

// TestImageContentBlockMarshalsToVisionSchema checks the image content block
// serializes to the exact Anthropic vision shape.
func TestImageContentBlockMarshalsToVisionSchema(t *testing.T) {
	block := UserContentBlock{
		Type:   "image",
		Source: &ImageSource{Type: "base64", MediaType: "image/png", Data: "aGk="},
	}
	got, err := json.Marshal(block)
	if err != nil {
		t.Fatal(err)
	}
	want := `{"type":"image","source":{"type":"base64","media_type":"image/png","data":"aGk="}}`
	if string(got) != want {
		t.Errorf("image block marshaled to\n  %s\nwant\n  %s", got, want)
	}
}

// TestSendWithImagesBuildsMultipartMessage checks a text+image turn produces the
// right content blocks, with the image base64-encoded.
func TestSendWithImagesBuildsMultipartMessage(t *testing.T) {
	s := &Stream{
		sendCh:  make(chan []UserContentBlock, 1),
		closeCh: make(chan struct{}),
	}
	err := s.SendWithImages(context.Background(), "look", []ImageInput{
		{Data: []byte("PNGDATA"), MediaType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}

	blocks := <-s.sendCh
	if len(blocks) != 2 {
		t.Fatalf("want 2 content blocks (text + image), got %d", len(blocks))
	}
	if blocks[0].Type != "text" || blocks[0].Text != "look" {
		t.Errorf("first block = %+v, want text %q", blocks[0], "look")
	}
	if blocks[1].Type != "image" || blocks[1].Source == nil {
		t.Fatalf("second block = %+v, want image with source", blocks[1])
	}
	if blocks[1].Source.MediaType != "image/png" {
		t.Errorf("media type = %q, want image/png", blocks[1].Source.MediaType)
	}
	if want := base64.StdEncoding.EncodeToString([]byte("PNGDATA")); blocks[1].Source.Data != want {
		t.Errorf("data = %q, want %q", blocks[1].Source.Data, want)
	}
}
