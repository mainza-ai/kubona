package llm

import (
	"errors"
	"testing"
)

func TestCloudPromptSecretScreenRefusesBeforeEgress(t *testing.T) {
	c := &Client{}
	err := c.screenCloudPrompt(ProviderOpenAI, []ChatMessage{
		{Role: "user", Content: "use this key sk-abcdefghijklmnopqrstuvwxyz123456"},
	})
	if err == nil {
		t.Fatal("cloud prompt with a credential shape must refuse")
	}
	var wireErr *WireError
	if !errors.As(err, &wireErr) || wireErr.Fault != FaultSecretInPrompt {
		t.Fatalf("fault = %v", err)
	}
}

func TestLocalPromptSecretScreenDoesNotBlock(t *testing.T) {
	c := &Client{}
	if err := c.screenCloudPrompt(ProviderOllama, []ChatMessage{
		{Role: "user", Content: "use this key sk-abcdefghijklmnopqrstuvwxyz123456"},
	}); err != nil {
		t.Fatalf("local provider must not be blocked by the cloud screen: %v", err)
	}
}
