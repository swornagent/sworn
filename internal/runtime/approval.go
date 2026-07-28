package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/swornagent/sworn/internal/baton"
)

const (
	approvalResolverVersion = "github-issue-comments/v1"
	maxApprovalResponse     = 4 * 1024 * 1024
	maxApprovalPages        = 10
)

// githubAPIBase is production-pinned. Real-binary E2E builds may replace this
// string at link time so fixture endpoints are unavailable in official builds.
var githubAPIBase = "https://api.github.com"

type approvalAdmission struct {
	planBytes  []byte
	evidence   []byte
	planDigest string
	reference  string
	commentID  int64
}

type approvalResolver interface {
	resolve(context.Context, admittedManifest, baton.Plan) (approvalAdmission, error)
}

type tokenSource func() (string, error)

type githubIssueApprovalResolver struct {
	baseURL string
	client  *http.Client
	token   tokenSource
}

func newProductionApprovalResolver(token tokenSource) approvalResolver {
	client := &http.Client{
		Timeout: 30 * time.Second,
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("redirects are disabled")
		},
	}
	return &githubIssueApprovalResolver{
		baseURL: githubAPIBase,
		client:  client,
		token:   token,
	}
}

func newFixtureApprovalResolver(baseURL string, client *http.Client) approvalResolver {
	if client == nil {
		return &githubIssueApprovalResolver{baseURL: baseURL}
	}
	closedClient := *client
	closedClient.CheckRedirect = func(*http.Request, []*http.Request) error {
		return errors.New("redirects are disabled")
	}
	return &githubIssueApprovalResolver{baseURL: baseURL, client: &closedClient}
}

type githubComment struct {
	ID                int64  `json:"id"`
	HTMLURL           string `json:"html_url"`
	Body              string `json:"body"`
	AuthorAssociation string `json:"author_association"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	User              struct {
		ID    int64  `json:"id"`
		Login string `json:"login"`
	} `json:"user"`
}

type matchedApprovalComment struct {
	comment githubComment
	etag    string
}

type approvalEvidence struct {
	SchemaVersion     string `json:"schema_version"`
	ResolverVersion   string `json:"resolver_version"`
	ApprovalRef       string `json:"approval_ref"`
	PlanDigest        string `json:"plan_digest"`
	MatchCount        int64  `json:"match_count"`
	Repository        string `json:"repository"`
	Issue             int64  `json:"issue"`
	CommentID         int64  `json:"comment_id"`
	URL               string `json:"url"`
	AuthorID          int64  `json:"author_id"`
	AuthorLogin       string `json:"author_login"`
	AuthorAssociation string `json:"author_association"`
	CreatedAt         string `json:"created_at"`
	UpdatedAt         string `json:"updated_at"`
	RawBodyDigest     string `json:"raw_body_digest"`
	Marker            string `json:"marker"`
	Decision          string `json:"decision"`
	ETag              string `json:"etag"`
}

func (r *githubIssueApprovalResolver) resolve(ctx context.Context, manifest admittedManifest,
	plan baton.Plan) (approvalAdmission, error) {
	if r == nil || r.client == nil || ctx == nil {
		return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
	}
	metadata := plan.Metadata()
	policy := manifest.value.Approval
	marker, err := approvalMarker(policy, metadata.ApprovalRef)
	if err != nil {
		return approvalAdmission{}, err
	}
	expectedRef := fmt.Sprintf(
		"github://%s/issues/%d#%s",
		policy.Repository,
		policy.Issue,
		marker,
	)
	if metadata.ApprovalRef != expectedRef ||
		metadata.Repository != policy.Repository ||
		metadata.Repository != manifest.value.Approval.Repository ||
		metadata.Release != manifest.value.Release ||
		metadata.TargetRef != manifest.value.TargetRef {
		return approvalAdmission{}, runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	base, err := url.Parse(r.baseURL)
	if err != nil || base.Scheme == "" || base.Host == "" {
		return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
	}
	parts := strings.Split(policy.Repository, "/")
	if len(parts) != 2 {
		return approvalAdmission{}, runtimeFail("INVALID_APPROVAL_POLICY", nil)
	}
	authorization := ""
	if r.token != nil {
		token, err := r.token()
		if err != nil || strings.ContainsAny(token, "\x00\r\n") {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		if token != "" {
			authorization = "Bearer " + token
		}
	}
	var (
		markerMatches int64
		valid         []matchedApprovalComment
	)
	for page := 1; page <= maxApprovalPages; page++ {
		endpoint := *base
		endpoint.Path = strings.TrimSuffix(base.Path, "/") +
			"/repos/" + url.PathEscape(parts[0]) + "/" + url.PathEscape(parts[1]) +
			"/issues/" + strconv.FormatInt(policy.Issue, 10) + "/comments"
		query := endpoint.Query()
		query.Set("per_page", "100")
		query.Set("page", strconv.Itoa(page))
		endpoint.RawQuery = query.Encode()
		request, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint.String(), nil)
		if err != nil {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		request.Header.Set("Accept", "application/vnd.github+json")
		request.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		request.Header.Set("User-Agent", "sworn/"+approvalResolverVersion)
		if authorization != "" {
			request.Header.Set("Authorization", authorization)
		}
		response, err := r.client.Do(request)
		if err != nil {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		if response.StatusCode != http.StatusOK {
			_ = response.Body.Close()
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		body, readErr := io.ReadAll(io.LimitReader(response.Body, maxApprovalResponse+1))
		closeErr := response.Body.Close()
		if readErr != nil || closeErr != nil || len(body) > maxApprovalResponse {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		if err := rejectDuplicateJSONKeys(body); err != nil {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		pageETag := response.Header.Get("ETag")
		var comments []githubComment
		decoder := json.NewDecoder(bytes.NewReader(body))
		if err := decoder.Decode(&comments); err != nil {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		if err := requireJSONEOF(decoder); err != nil {
			return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
		}
		for _, comment := range comments {
			if !strings.Contains(comment.Body, marker) {
				continue
			}
			markerMatches++
			if validApprovalComment(comment, policy, marker, plan.Digest()) {
				valid = append(valid, matchedApprovalComment{
					comment: comment,
					etag:    pageETag,
				})
			}
		}
		if len(comments) < 100 {
			break
		}
		if page == maxApprovalPages {
			return approvalAdmission{}, runtimeFail("APPROVAL_RESOURCE_LIMIT", nil)
		}
	}
	if markerMatches != 1 || len(valid) != 1 {
		if markerMatches == 0 {
			return approvalAdmission{}, runtimeFail("APPROVAL_PENDING", nil)
		}
		return approvalAdmission{}, runtimeFail("APPROVAL_AMBIGUOUS", nil)
	}
	comment := valid[0].comment
	evidence := approvalEvidence{
		SchemaVersion:     "sworn.approval-evidence/v1",
		ResolverVersion:   approvalResolverVersion,
		ApprovalRef:       expectedRef,
		PlanDigest:        plan.Digest(),
		MatchCount:        markerMatches,
		Repository:        policy.Repository,
		Issue:             policy.Issue,
		CommentID:         comment.ID,
		URL:               comment.HTMLURL,
		AuthorID:          comment.User.ID,
		AuthorLogin:       comment.User.Login,
		AuthorAssociation: comment.AuthorAssociation,
		CreatedAt:         comment.CreatedAt,
		UpdatedAt:         comment.UpdatedAt,
		RawBodyDigest:     sha256Digest([]byte(comment.Body)),
		Marker:            marker,
		Decision:          "approved",
		ETag:              valid[0].etag,
	}
	evidenceBody, err := json.Marshal(evidence)
	if err != nil {
		return approvalAdmission{}, runtimeFail("APPROVAL_UNAVAILABLE", nil)
	}
	return approvalAdmission{
		planBytes:  plan.Bytes(),
		evidence:   append(evidenceBody, '\n'),
		planDigest: plan.Digest(),
		reference:  expectedRef,
		commentID:  comment.ID,
	}, nil
}

func validApprovalComment(comment githubComment, policy ApprovalPolicy,
	marker, planDigest string) bool {
	if comment.ID < 1 ||
		comment.User.ID < 1 ||
		!containsInt64(policy.AllowedAuthorIDs, comment.User.ID) ||
		!containsString(policy.AllowedAssociations, comment.AuthorAssociation) ||
		comment.User.Login == "" ||
		strings.ContainsAny(comment.User.Login, "\x00\r\n") ||
		comment.CreatedAt == "" ||
		comment.CreatedAt != comment.UpdatedAt {
		return false
	}
	if _, err := time.Parse(time.RFC3339, comment.CreatedAt); err != nil {
		return false
	}
	wantBody := fmt.Sprintf(
		"baton-plan-approval/v1\nmarker: %s\ndecision: approved\nrepository: %s\nissue: %d\nplan_digest: %s\n",
		marker,
		policy.Repository,
		policy.Issue,
		planDigest,
	)
	if comment.Body != wantBody {
		return false
	}
	parsed, err := url.Parse(comment.HTMLURL)
	if err != nil || parsed.Scheme != "https" || parsed.Host != "github.com" ||
		parsed.User != nil || parsed.Opaque != "" || parsed.RawPath != "" ||
		parsed.RawQuery != "" || parsed.ForceQuery ||
		parsed.Path != "/"+policy.Repository+"/issues/"+strconv.FormatInt(policy.Issue, 10) ||
		parsed.Fragment != "issuecomment-"+strconv.FormatInt(comment.ID, 10) {
		return false
	}
	return true
}

// validatePersistedApprovalEvidence re-admits the durable approval envelope
// against the run's manifest policy. The journal is recovery input, not an
// authority source: every fact that the live resolver checked must still be
// derivable from the canonical evidence before an install effect can be
// replayed or recognized as complete.
func validatePersistedApprovalEvidence(
	manifest admittedManifest,
	plan baton.Plan,
	input installActionInput,
	evidence approvalEvidence,
) error {
	metadata := plan.Metadata()
	policy := manifest.value.Approval
	marker, err := approvalMarker(policy, metadata.ApprovalRef)
	if err != nil {
		return runtimeFail("CORRUPT_JOURNAL", err)
	}
	expectedRef := fmt.Sprintf(
		"github://%s/issues/%d#%s",
		policy.Repository,
		policy.Issue,
		marker,
	)
	expectedBody := fmt.Sprintf(
		"baton-plan-approval/v1\nmarker: %s\ndecision: approved\nrepository: %s\nissue: %d\nplan_digest: %s\n",
		marker,
		policy.Repository,
		policy.Issue,
		plan.Digest(),
	)
	comment := githubComment{
		ID:                evidence.CommentID,
		HTMLURL:           evidence.URL,
		Body:              expectedBody,
		AuthorAssociation: evidence.AuthorAssociation,
		CreatedAt:         evidence.CreatedAt,
		UpdatedAt:         evidence.UpdatedAt,
	}
	comment.User.ID = evidence.AuthorID
	comment.User.Login = evidence.AuthorLogin
	if evidence.SchemaVersion != "sworn.approval-evidence/v1" ||
		evidence.ResolverVersion != approvalResolverVersion ||
		evidence.ApprovalRef != expectedRef ||
		evidence.ApprovalRef != input.Reference ||
		evidence.PlanDigest != input.PlanDigest ||
		evidence.MatchCount != 1 ||
		evidence.Repository != policy.Repository ||
		evidence.Issue != policy.Issue ||
		evidence.CommentID != input.CommentID ||
		evidence.Marker != marker ||
		evidence.Decision != "approved" ||
		evidence.RawBodyDigest != sha256Digest([]byte(expectedBody)) ||
		strings.ContainsAny(evidence.ETag, "\x00\r\n") ||
		metadata.Repository != policy.Repository ||
		metadata.Release != manifest.value.Release ||
		metadata.TargetRef != manifest.value.TargetRef ||
		!validApprovalComment(
			comment,
			policy,
			marker,
			plan.Digest(),
		) {
		return runtimeFail("CORRUPT_JOURNAL", nil)
	}
	return nil
}

func approvalMarker(policy ApprovalPolicy, reference string) (string, error) {
	prefix := fmt.Sprintf("github://%s/issues/%d#", policy.Repository, policy.Issue)
	marker := strings.TrimPrefix(reference, prefix)
	if marker == reference || !markerPattern.MatchString(marker) {
		return "", runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	return marker, nil
}

type authorityInstaller struct {
	actions *baton.Actions
}

func newAuthorityInstaller(actions *baton.Actions) *authorityInstaller {
	return &authorityInstaller{actions: actions}
}

// install is the only runtime path authorized to call RecordPlanRevision.
func (i *authorityInstaller) install(admission approvalAdmission) (baton.ActionResult, error) {
	if i == nil || i.actions == nil ||
		admission.planDigest == "" ||
		sha256Digest(admission.planBytes) != admission.planDigest ||
		len(admission.evidence) == 0 {
		return baton.ActionResult{}, runtimeFail("APPROVAL_ADMISSION_REQUIRED", nil)
	}
	return i.actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: admission.planBytes,
		Summary:   "Install the exact externally approved plan.",
		Detail:    admission.evidence,
	})
}
