package comments

import (
	"fmt"
	"io"
	"strings"

	"github.com/antiwork/gumroad-cli/internal/admincmd"
	"github.com/antiwork/gumroad-cli/internal/cmd/admin/users/usertarget"
	"github.com/antiwork/gumroad-cli/internal/cmdutil"
	"github.com/antiwork/gumroad-cli/internal/cmdutil/cursor"
	"github.com/antiwork/gumroad-cli/internal/output"
	"github.com/spf13/cobra"
)

const commentPreviewLimit = 200

type listResponse struct {
	UserID     string            `json:"user_id"`
	Comments   []commentPayload  `json:"comments"`
	Pagination cursor.Pagination `json:"pagination"`
}

func newListCmd() *cobra.Command {
	var (
		lookup usertarget.LookupFlags
		page   cursor.Flags
		types  []string
	)

	cmd := &cobra.Command{
		Use:   "list",
		Short: "List admin comments for a user",
		Example: `  gumroad admin users comments list --user-id 2245593582708
  gumroad admin users comments list --username sellerone --type note
  gumroad admin users comments list --email user@example.com --type note --type suspension_note --limit 50
  gumroad admin users comments list --email user@example.com --cursor cur-next`,
		Args: cmdutil.ExactArgs(0),
		RunE: func(c *cobra.Command, args []string) error {
			opts := cmdutil.OptionsFrom(c)
			target, err := usertarget.ResolveLookupTarget(c, lookup)
			if err != nil {
				return err
			}
			if err := cmdutil.RequirePositiveIntFlag(c, "limit", page.Limit); err != nil {
				return err
			}

			params := target.Values()
			if len(types) > 0 {
				params.Set("comment_type", strings.Join(types, ","))
			}
			cursor.Apply(params, page)

			return admincmd.RunGetDecoded[listResponse](opts, "Fetching user comments...", "/users/comments", params, func(resp listResponse) error {
				return renderList(opts, target.Identifier(), resp)
			})
		},
	}

	usertarget.AddLookupFlags(cmd, &lookup)
	cmd.Flags().StringArrayVar(&types, "type", nil, "Comment type filter (repeatable)")
	cursor.AddFlags(cmd, &page)

	return cmd
}

func renderList(opts cmdutil.Options, identifier string, resp listResponse) error {
	if opts.PlainOutput {
		return writeCommentsPlain(opts.Out(), resp.Comments)
	}

	if opts.Quiet {
		return nil
	}

	style := opts.Style()
	return output.WithPager(opts.Out(), opts.Err(), func(w io.Writer) error {
		if len(resp.Comments) == 0 {
			if err := output.Writef(w, "No comments found for %s.\n", identifier); err != nil {
				return err
			}
			return cursor.WriteMoreFooter(w, resp.Pagination)
		}

		headline := fmt.Sprintf("%d comment(s) for %s", len(resp.Comments), identifier)
		if err := output.Writeln(w, style.Bold(headline)); err != nil {
			return err
		}
		if resp.UserID != "" && resp.UserID != identifier {
			if err := output.Writef(w, "User ID: %s\n", resp.UserID); err != nil {
				return err
			}
		}

		for _, comment := range resp.Comments {
			if err := output.Writeln(w, ""); err != nil {
				return err
			}
			if err := writeCommentBlock(w, style, comment); err != nil {
				return err
			}
		}

		return cursor.WriteMoreFooter(w, resp.Pagination)
	})
}

func writeCommentsPlain(w io.Writer, comments []commentPayload) error {
	rows := make([][]string, 0, len(comments))
	for _, comment := range comments {
		rows = append(rows, []string{
			comment.ID,
			commentTypeLabel(comment),
			commentAuthorLabel(comment),
			comment.CreatedAt,
			comment.DeletedAt,
			comment.Content,
		})
	}
	return output.PrintPlain(w, rows)
}

func writeCommentBlock(w io.Writer, style output.Styler, comment commentPayload) error {
	titleParts := []string{comment.ID, commentTypeLabel(comment)}
	if comment.DeletedAt != "" {
		titleParts = append(titleParts, "(deleted)")
	}
	title := strings.Join(nonEmptyStrings(titleParts...), " ")
	if title == "" {
		title = "Comment"
	}
	if err := output.Writeln(w, style.Bold(title)); err != nil {
		return err
	}
	if author := commentAuthorLabel(comment); author != "" {
		if err := output.Writef(w, "Author: %s\n", author); err != nil {
			return err
		}
	}
	if comment.CreatedAt != "" {
		if err := output.Writef(w, "Created: %s\n", comment.CreatedAt); err != nil {
			return err
		}
	}
	if comment.DeletedAt != "" {
		if err := output.Writef(w, "Deleted: %s\n", comment.DeletedAt); err != nil {
			return err
		}
	}
	if err := output.Writeln(w, "Content:"); err != nil {
		return err
	}
	return output.Writeln(w, truncateCommentContent(comment.Content, commentPreviewLimit))
}

func commentTypeLabel(comment commentPayload) string {
	return usertarget.Fallback(comment.CommentType, comment.Type)
}

func commentAuthorLabel(comment commentPayload) string {
	if comment.AuthorName != "" {
		return comment.AuthorName
	}

	parts := make([]string, 0, 3)
	if comment.Author.Name != "" {
		parts = append(parts, comment.Author.Name)
	}
	if comment.Author.Email != "" && comment.Author.Email != comment.Author.Name {
		parts = append(parts, comment.Author.Email)
	}
	if comment.Author.ID != "" {
		parts = append(parts, comment.Author.ID)
	}
	return strings.Join(parts, " / ")
}

func truncateCommentContent(value string, limit int) string {
	runes := []rune(value)
	if len(runes) <= limit {
		return value
	}
	return string(runes[:limit]) + "..."
}

func nonEmptyStrings(values ...string) []string {
	parts := make([]string, 0, len(values))
	for _, value := range values {
		if value != "" {
			parts = append(parts, value)
		}
	}
	return parts
}
