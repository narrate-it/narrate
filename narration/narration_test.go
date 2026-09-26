package narration_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/narrate-it/narrate/narration"
)

type rewriteFunc func(context.Context, string, string) (string, error)

func (f rewriteFunc) Rewrite(ctx context.Context, instructions, source string) (string, error) {
	return f(ctx, instructions, source)
}

func TestHostControlledRewrite(t *testing.T) {
	calls := 0
	r := narration.Rewriter{
		Style: "agent-update", DisableRetries: true,
		Client: rewriteFunc(func(ctx context.Context, instructions, source string) (string, error) {
			calls++
			if !strings.Contains(instructions, "[A1]") || !strings.Contains(source, "The build failed.") {
				t.Fatal("missing canonical craft or source")
			}
			return "The build failed.", nil
		}),
	}
	parts, err := r.RewriteAll(context.Background(), []narration.Chunk{{Text: "The build failed.", First: true, Last: true}}, nil)
	if err != nil || len(parts) != 1 || parts[0] != "The build failed." || calls != 1 {
		t.Fatalf("parts=%v calls=%d err=%v", parts, calls, err)
	}
}

func TestHostCanPreventUnadmittedRepair(t *testing.T) {
	calls := 0
	r := narration.Rewriter{Style: "agent-update", DisableRetries: true,
		Client: rewriteFunc(func(context.Context, string, string) (string, error) {
			calls++
			return "", nil
		}),
	}
	_, err := r.RewriteAll(context.Background(), []narration.Chunk{{Text: "A result."}}, nil)
	if !errors.Is(err, narration.ErrTruncated) || calls != 1 {
		t.Fatalf("calls=%d err=%v", calls, err)
	}
}

func TestPublicCraftAssets(t *testing.T) {
	for _, style := range []string{"conversational", "coach", "agent-update"} {
		prompt, err := narration.Assemble(style)
		if err != nil || !strings.Contains(prompt, "[F1]") {
			t.Fatalf("style=%s err=%v", style, err)
		}
		digest, err := narration.EffectiveDigest(style)
		if err != nil || len(digest) != 64 {
			t.Fatalf("style=%s digest=%s err=%v", style, digest, err)
		}
	}
	if _, err := narration.Assemble("unknown"); err == nil {
		t.Fatal("accepted unknown style")
	}
	manifest, err := narration.Manifest()
	if err != nil || manifest.Version != narration.PromptVersion || len(manifest.Modules) != 9 {
		t.Fatalf("manifest=%v err=%v", manifest, err)
	}
}
