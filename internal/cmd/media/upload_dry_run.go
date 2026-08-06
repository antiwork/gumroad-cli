package media

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/antiwork/gumroad-cli/internal/uploadui"
)

type dryRunUploadRequest struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Params  map[string]string `json:"params,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    string            `json:"body,omitempty"`
}

type dryRunUploadPlan struct {
	DryRun      bool                  `json:"dry_run"`
	Action      string                `json:"action"`
	Path        string                `json:"path"`
	Filename    string                `json:"filename"`
	Name        string                `json:"name,omitempty"`
	ContentType string                `json:"content_type"`
	Checksum    string                `json:"checksum"`
	Size        int64                 `json:"size"`
	Requests    []dryRunUploadRequest `json:"requests"`
}

func buildDryRunUploadPlan(plan plannedMediaUpload, name string) dryRunUploadPlan {
	reserveParams := map[string]string{
		"purpose":            "media",
		"blob[filename]":     plan.Filename,
		"blob[byte_size]":    strconv.FormatInt(plan.Size, 10),
		"blob[checksum]":     plan.Checksum,
		"blob[content_type]": plan.ContentType,
	}
	commitParams := map[string]string{"signed_blob_id": "<signed_blob_id>"}
	if name != "" {
		commitParams["name"] = name
	}

	return dryRunUploadPlan{
		DryRun:      true,
		Action:      "media upload",
		Path:        plan.Path,
		Filename:    plan.Filename,
		Name:        name,
		ContentType: plan.ContentType,
		Checksum:    plan.Checksum,
		Size:        plan.Size,
		Requests: []dryRunUploadRequest{
			{
				Method: http.MethodPost,
				Path:   "/direct_uploads",
				Params: reserveParams,
			},
			{
				Method: http.MethodPut,
				Path:   "<direct_upload_url>",
				Headers: map[string]string{
					"Content-MD5":  plan.Checksum,
					"Content-Type": plan.ContentType,
				},
				Body: "<contents of " + plan.Path + ">",
			},
			{
				Method: http.MethodPost,
				Path:   "/media",
				Params: commitParams,
			},
		},
	}
}

func renderUploadDryRun(opts cmdutil.Options, plan plannedMediaUpload, name string) error {
	dryRun := buildDryRunUploadPlan(plan, name)
	if opts.UsesJSONOutput() {
		data, err := json.Marshal(dryRun)
		if err != nil {
			return err
		}
		return output.PrintJSON(opts.Out(), data, opts.JQExpr)
	}
	if opts.PlainOutput {
		rows := [][]string{{dryRun.Action, dryRun.Path, dryRun.ContentType, strconv.FormatInt(dryRun.Size, 10), dryRun.Name}}
		for _, request := range dryRun.Requests {
			params := url.Values{}
			for key, value := range request.Params {
				params.Set(key, value)
			}
			headers, err := json.Marshal(request.Headers)
			if err != nil {
				return err
			}
			rows = append(rows, []string{request.Method, request.Path, params.Encode(), string(headers), request.Body})
		}
		return output.PrintPlain(opts.Out(), rows)
	}
	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	if err := output.Writeln(opts.Out(), style.Bold("Dry run: media upload")); err != nil {
		return err
	}
	if err := output.Writeln(opts.Out(), "File: "+dryRun.Path); err != nil {
		return err
	}
	if dryRun.Name != "" {
		if err := output.Writeln(opts.Out(), "Display name: "+dryRun.Name); err != nil {
			return err
		}
	}
	if err := output.Writeln(opts.Out(), "Content type: "+dryRun.ContentType); err != nil {
		return err
	}
	if err := output.Writeln(opts.Out(), "Size: "+uploadui.HumanBytes(dryRun.Size)); err != nil {
		return err
	}
	for _, request := range dryRun.Requests {
		if err := output.Writeln(opts.Out(), fmt.Sprintf("Request: %s %s", request.Method, request.Path)); err != nil {
			return err
		}
		if len(request.Params) > 0 {
			params, err := json.Marshal(request.Params)
			if err != nil {
				return err
			}
			if err := output.Writeln(opts.Out(), "  Params: "+string(params)); err != nil {
				return err
			}
		}
		if len(request.Headers) > 0 {
			headers, err := json.Marshal(request.Headers)
			if err != nil {
				return err
			}
			if err := output.Writeln(opts.Out(), "  Headers: "+string(headers)); err != nil {
				return err
			}
		}
		if request.Body != "" {
			if err := output.Writeln(opts.Out(), "  Body: "+request.Body); err != nil {
				return err
			}
		}
	}
	return nil
}
