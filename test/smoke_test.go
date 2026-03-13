package test

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

func TestSmoke_UserAndAuthStatus(t *testing.T) {
	if os.Getenv("GUMROAD_SMOKE") != "1" {
		t.Skip("set GUMROAD_SMOKE=1 and GUMROAD_ACCESS_TOKEN to run live Gumroad smoke tests")
	}

	token := os.Getenv("GUMROAD_ACCESS_TOKEN")
	if token == "" {
		t.Fatal("GUMROAD_ACCESS_TOKEN is required when GUMROAD_SMOKE=1")
	}

	bin := buildBinary(t)
	cfgDir := t.TempDir()
	env := []string{"XDG_CONFIG_HOME=" + cfgDir}
	if baseURL := os.Getenv("GUMROAD_API_BASE_URL"); baseURL != "" {
		env = append(env, "GUMROAD_API_BASE_URL="+baseURL)
	}

	loginOut, err := runGRWithInput(t, bin, env, token+"\n", "auth", "login", "--json")
	if err != nil {
		t.Fatalf("gumroad auth login --json failed: %v\n%s", err, loginOut)
	}
	var loginPayload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(loginOut), &loginPayload); err != nil {
		t.Fatalf("gumroad auth login --json output is not valid JSON: %v\n%s", err, loginOut)
	}
	var loginResp struct {
		Authenticated bool `json:"authenticated"`
	}
	mustDecodeSmokePayload(t, loginPayload, &loginResp)
	if !loginResp.Authenticated {
		t.Fatalf("expected authenticated login response, got %v", loginPayload)
	}

	assertSmokeJSON(t, bin, env, []string{"user", "--json"}, func(payload map[string]json.RawMessage) {
		var userResp struct {
			User map[string]any `json:"user"`
		}
		mustDecodeSmokePayload(t, payload, &userResp)
		if len(userResp.User) == 0 {
			t.Fatalf("expected user payload, got %v", payload)
		}
	})

	assertSmokeJSON(t, bin, env, []string{"auth", "status", "--json"}, func(payload map[string]json.RawMessage) {
		var statusResp struct {
			Authenticated bool `json:"authenticated"`
		}
		mustDecodeSmokePayload(t, payload, &statusResp)
		if !statusResp.Authenticated {
			t.Fatalf("expected authenticated status, got %v", payload)
		}
	})

	assertSmokePlainNonEmpty(t, bin, env, []string{"auth", "status", "--plain"})
	email := assertSmokeJSONValue[string](t, bin, env, []string{"user", "--json", "--jq", ".user.email"})
	if strings.TrimSpace(email) == "" {
		t.Fatal("expected user email from jq output")
	}

	productsPayload := assertSmokeJSONWithKeys(t, bin, env, []string{"products", "list", "--json"}, "products")
	productCount := assertSmokeJSONValue[int](t, bin, env, []string{"products", "list", "--json", "--jq", ".products | length"})
	if productCount < 0 {
		t.Fatalf("expected non-negative product count, got %d", productCount)
	}
	productID := firstIDFromPayload(t, productsPayload, "products")
	if productID != "" {

		assertSmokeJSONWithKeys(t, bin, env, []string{"products", "view", productID, "--json"}, "product")

		categoryPayload := assertSmokeJSONWithKeys(t, bin, env, []string{"variant-categories", "list", "--product", productID, "--json"}, "variant_categories")
		assertSmokeJSONWithKeys(t, bin, env, []string{"offer-codes", "list", "--product", productID, "--json"}, "offer_codes")
		assertSmokeJSONWithKeys(t, bin, env, []string{"custom-fields", "list", "--product", productID, "--json"}, "custom_fields")

		categoryID := firstIDFromPayload(t, categoryPayload, "variant_categories")
		if categoryID != "" {
			assertSmokeJSONWithKeys(t, bin, env, []string{"variants", "list", "--product", productID, "--category", categoryID, "--json"}, "variants")
		}

		subscribersPayload := assertSmokeJSONWithKeys(t, bin, env, []string{"subscribers", "list", "--product", productID, "--json"}, "subscribers")
		subscriberID := firstIDFromPayload(t, subscribersPayload, "subscribers")
		if subscriberID != "" {
			assertSmokeJSONWithKeys(t, bin, env, []string{"subscribers", "view", subscriberID, "--json"}, "subscriber")
		}
	}

	salesPayload := assertSmokeJSONWithKeys(t, bin, env, []string{"sales", "list", "--json"}, "sales")
	saleCount := assertSmokeJSONValue[int](t, bin, env, []string{"sales", "list", "--json", "--jq", ".sales | length"})
	if saleCount < 0 {
		t.Fatalf("expected non-negative sale count, got %d", saleCount)
	}
	saleID := firstIDFromPayload(t, salesPayload, "sales")
	if saleID != "" {
		assertSmokeJSONWithKeys(t, bin, env, []string{"sales", "view", saleID, "--json"}, "sale")
	}

	payoutsPayload := assertSmokeJSONWithKeys(t, bin, env, []string{"payouts", "list", "--json"}, "payouts")
	assertSmokeJSONWithKeys(t, bin, env, []string{"payouts", "upcoming", "--json"}, "payout")
	payoutID := firstIDFromPayload(t, payoutsPayload, "payouts")
	if payoutID != "" {
		assertSmokeJSONWithKeys(t, bin, env, []string{"payouts", "view", payoutID, "--json"}, "payout")
	}

	assertSmokeJSONWithKeys(t, bin, env, []string{"webhooks", "list", "--resource", "sale", "--json"}, "resource_subscriptions")

	assertSmokeJSON(t, bin, env, []string{"auth", "logout", "--yes", "--json"}, func(payload map[string]json.RawMessage) {
		var logoutResp struct {
			LoggedOut bool `json:"logged_out"`
		}
		mustDecodeSmokePayload(t, payload, &logoutResp)
		if !logoutResp.LoggedOut {
			t.Fatalf("expected logged_out status, got %v", payload)
		}
	})
}

func assertSmokeJSON(t *testing.T, bin string, env []string, args []string, assert func(map[string]json.RawMessage)) map[string]json.RawMessage {
	t.Helper()

	command := "gumroad " + strings.Join(args, " ")
	out, err := runGR(t, bin, env, args...)
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", command, err, out)
	}

	var payload map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &payload); err != nil {
		t.Fatalf("%s output is not valid JSON: %v\n%s", command, err, out)
	}

	assert(payload)
	return payload
}

func assertSmokeJSONWithKeys(t *testing.T, bin string, env []string, args []string, keys ...string) map[string]json.RawMessage {
	t.Helper()

	return assertSmokeJSON(t, bin, env, args, func(payload map[string]json.RawMessage) {
		assertSmokeKeys(t, payload, keys...)
	})
}

func assertSmokeJSONValue[T any](t *testing.T, bin string, env []string, args []string) T {
	t.Helper()

	command := "gumroad " + strings.Join(args, " ")
	out, err := runGR(t, bin, env, args...)
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", command, err, out)
	}

	var value T
	if err := json.Unmarshal([]byte(out), &value); err != nil {
		t.Fatalf("%s output is not valid JSON: %v\n%s", command, err, out)
	}
	return value
}

func assertSmokePlainNonEmpty(t *testing.T, bin string, env []string, args []string) string {
	t.Helper()

	command := "gumroad " + strings.Join(args, " ")
	out, err := runGR(t, bin, env, args...)
	if err != nil {
		t.Fatalf("%s failed: %v\n%s", command, err, out)
	}
	if strings.TrimSpace(out) == "" {
		t.Fatalf("%s output was empty", command)
	}
	return out
}

func assertSmokeKeys(t *testing.T, payload map[string]json.RawMessage, keys ...string) {
	t.Helper()

	for _, key := range keys {
		if _, ok := payload[key]; !ok {
			t.Fatalf("missing %q in %v", key, payload)
		}
	}
}

func firstIDFromPayload(t *testing.T, payload map[string]json.RawMessage, key string) string {
	t.Helper()

	data, ok := payload[key]
	if !ok {
		t.Fatalf("missing %q in %v", key, payload)
	}

	var items []struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(data, &items); err != nil {
		t.Fatalf("decode %q items failed: %v", key, err)
	}
	if len(items) == 0 {
		return ""
	}
	return items[0].ID
}

func mustDecodeSmokePayload(t *testing.T, payload map[string]json.RawMessage, target any) {
	t.Helper()

	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal payload failed: %v", err)
	}
	if err := json.Unmarshal(data, target); err != nil {
		t.Fatalf("decode payload failed: %v", err)
	}
}
