package baton

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

const (
	PlanVersion = "baton.plan/v2"
	planOpen    = "```baton-plan-v2\n"
	planClose   = "\n```\n"
	RecordRoot  = ".baton/releases"
)

var (
	identityPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	objectIDPattern = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	digestPattern   = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
)

type Criterion struct {
	ID   string `json:"id"`
	Text string `json:"text"`
}

type Scope struct {
	Include []string `json:"include"`
	Exclude []string `json:"exclude"`
}

type Slice struct {
	ID          string      `json:"id"`
	Outcome     string      `json:"outcome"`
	Scope       Scope       `json:"scope"`
	Acceptance  []Criterion `json:"acceptance"`
	Checks      []string    `json:"checks"`
	Constraints []string    `json:"constraints"`
	DependsOn   []string    `json:"depends_on"`
	Consumes    []string    `json:"consumes"`
}

type Track struct {
	ID        string   `json:"id"`
	DependsOn []string `json:"depends_on"`
	Slices    []Slice  `json:"slices"`
}

type Metadata struct {
	SchemaVersion string            `json:"schema_version"`
	Release       string            `json:"release"`
	Revision      int64             `json:"revision"`
	PreviousPlan  *string           `json:"previous_plan"`
	Repository    string            `json:"repository"`
	TargetRef     string            `json:"target_ref"`
	ApprovalRef   string            `json:"approval_ref"`
	Tracks        []Track           `json:"tracks"`
	Contracts     map[string]string `json:"-"`
}

type planAdmission struct {
	raw      []byte
	digest   string
	metadata Metadata
	markdown string
}

// Plan is an immutable admission of exact baton.plan/v2 bytes.
type Plan struct {
	admission *planAdmission
}

func DigestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}

func ParsePlan(raw []byte) (Plan, error) {
	if len(raw) > MaxPlanBytes {
		return Plan{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("plan exceeds %d bytes", MaxPlanBytes))
	}
	if !bytes.HasPrefix(raw, []byte(planOpen)) {
		return Plan{}, recordFail("INVALID_PLAN_FENCE", "plan must begin at byte zero")
	}
	body := raw[len(planOpen):]
	closeAt := bytes.Index(body, []byte(planClose))
	if closeAt < 0 || bytes.Index(body[closeAt+len(planClose):], []byte(planClose)) >= 0 {
		return Plan{}, recordFail("INVALID_PLAN_FENCE", "plan must contain one closed baton-plan-v2 block")
	}
	metadataValue, err := strictParseJSON(body[:closeAt], "plan metadata", MaxPlanBytes)
	if err != nil {
		return Plan{}, err
	}
	object, err := asObject(metadataValue, "plan")
	if err != nil {
		return Plan{}, err
	}
	metadata, err := validatePlanMetadata(object)
	if err != nil {
		return Plan{}, err
	}
	markdownBytes := body[closeAt+len(planClose):]
	if !utf8.Valid(markdownBytes) {
		return Plan{}, recordFail("INVALID_UTF8", "plan Markdown is not valid UTF-8")
	}
	copied := append([]byte(nil), raw...)
	return Plan{admission: &planAdmission{
		raw: copied, digest: DigestBytes(copied),
		metadata: metadata, markdown: string(markdownBytes),
	}}, nil
}

func (p Plan) require() (*planAdmission, error) {
	if p.admission == nil || !digestPattern.MatchString(p.admission.digest) ||
		DigestBytes(p.admission.raw) != p.admission.digest {
		return nil, recordFail("PLAN_ADMISSION_REQUIRED", "operation requires one immutable parsed plan")
	}
	return p.admission, nil
}

func (p Plan) Digest() string {
	if p.admission == nil {
		return ""
	}
	return p.admission.digest
}

func (p Plan) Bytes() []byte {
	if p.admission == nil {
		return nil
	}
	return append([]byte(nil), p.admission.raw...)
}

func (p Plan) Markdown() string {
	if p.admission == nil {
		return ""
	}
	return p.admission.markdown
}

func (p Plan) Metadata() Metadata {
	if p.admission == nil {
		return Metadata{}
	}
	return copyMetadata(p.admission.metadata)
}

func copyMetadata(value Metadata) Metadata {
	result := value
	if value.PreviousPlan != nil {
		previous := *value.PreviousPlan
		result.PreviousPlan = &previous
	}
	result.Contracts = make(map[string]string, len(value.Contracts))
	for key, digest := range value.Contracts {
		result.Contracts[key] = digest
	}
	result.Tracks = make([]Track, len(value.Tracks))
	for i, track := range value.Tracks {
		result.Tracks[i] = track
		result.Tracks[i].DependsOn = cloneStrings(track.DependsOn)
		result.Tracks[i].Slices = make([]Slice, len(track.Slices))
		for j, slice := range track.Slices {
			result.Tracks[i].Slices[j] = slice
			result.Tracks[i].Slices[j].Scope.Include = cloneStrings(slice.Scope.Include)
			result.Tracks[i].Slices[j].Scope.Exclude = cloneStrings(slice.Scope.Exclude)
			result.Tracks[i].Slices[j].Acceptance = append([]Criterion(nil), slice.Acceptance...)
			result.Tracks[i].Slices[j].Checks = cloneStrings(slice.Checks)
			result.Tracks[i].Slices[j].Constraints = cloneStrings(slice.Constraints)
			result.Tracks[i].Slices[j].DependsOn = cloneStrings(slice.DependsOn)
			result.Tracks[i].Slices[j].Consumes = cloneStrings(slice.Consumes)
		}
	}
	return result
}

func cloneStrings(value []string) []string {
	if value == nil {
		return nil
	}
	return append([]string{}, value...)
}

func (p Plan) FindTrack(id string) (Track, bool) {
	if p.admission == nil {
		return Track{}, false
	}
	for _, track := range p.admission.metadata.Tracks {
		if track.ID == id {
			copy := copyMetadata(Metadata{Tracks: []Track{track}}).Tracks[0]
			return copy, true
		}
	}
	return Track{}, false
}

func (p Plan) FindSlice(id string) (Track, Slice, bool) {
	if p.admission == nil {
		return Track{}, Slice{}, false
	}
	for _, track := range p.admission.metadata.Tracks {
		for _, slice := range track.Slices {
			if slice.ID == id {
				copyTrack := copyMetadata(Metadata{Tracks: []Track{track}}).Tracks[0]
				for _, copied := range copyTrack.Slices {
					if copied.ID == id {
						return copyTrack, copied, true
					}
				}
			}
		}
	}
	return Track{}, Slice{}, false
}

func (p Plan) Contract(id string) (string, bool) {
	if p.admission == nil {
		return "", false
	}
	value, ok := p.admission.metadata.Contracts[id]
	return value, ok
}

func validatePlanMetadata(value map[string]any) (Metadata, error) {
	required := []string{
		"schema_version", "release", "revision", "previous_plan",
		"repository", "target_ref", "approval_ref", "tracks",
	}
	if err := exactKeys(value, required, nil, "plan"); err != nil {
		return Metadata{}, err
	}
	schema, err := requiredString(value["schema_version"], "plan.schema_version", 1, 100)
	if err != nil {
		return Metadata{}, err
	}
	if schema != PlanVersion {
		return Metadata{}, recordFail("INVALID_FIELD", "plan.schema_version must be "+PlanVersion)
	}
	release, err := identity(value["release"], "plan.release")
	if err != nil {
		return Metadata{}, err
	}
	revision, err := safeInteger(value["revision"], "plan.revision", 1)
	if err != nil {
		return Metadata{}, err
	}
	var previous *string
	if value["previous_plan"] != nil {
		oid, err := objectID(value["previous_plan"], "plan.previous_plan")
		if err != nil {
			return Metadata{}, err
		}
		previous = &oid
	}
	if (revision == 1) != (previous == nil) {
		return Metadata{}, recordFail("INVALID_FIELD", "plan revision 1 alone must use previous_plan null")
	}
	repository, err := requiredString(value["repository"], "plan.repository", 1, 256)
	if err != nil {
		return Metadata{}, err
	}
	if strings.HasPrefix(repository, "/") || strings.HasPrefix(repository, "\\") ||
		(len(repository) >= 3 && ((repository[0] >= 'A' && repository[0] <= 'Z') ||
			(repository[0] >= 'a' && repository[0] <= 'z')) && repository[1] == ':' &&
			(repository[2] == '/' || repository[2] == '\\')) {
		return Metadata{}, recordFail("INVALID_FIELD", "plan.repository must be a portable identity")
	}
	targetRef, err := headRef(value["target_ref"], "plan.target_ref")
	if err != nil {
		return Metadata{}, err
	}
	approvalRef, err := requiredString(value["approval_ref"], "plan.approval_ref", 1, 500)
	if err != nil {
		return Metadata{}, err
	}
	rawTracks, err := asArray(value["tracks"], "plan.tracks", true, MaxTracks)
	if err != nil {
		return Metadata{}, err
	}

	tracks := make([]Track, 0, len(rawTracks))
	trackIDs := make(map[string]bool, len(rawTracks))
	sliceIDs := make(map[string]bool)
	contracts := make(map[string]string)
	totalSlices := 0
	for trackIndex, rawTrack := range rawTracks {
		label := fmt.Sprintf("plan.tracks[%d]", trackIndex)
		object, err := asObject(rawTrack, label)
		if err != nil {
			return Metadata{}, err
		}
		if err := exactKeys(object, []string{"id", "depends_on", "slices"}, nil, label); err != nil {
			return Metadata{}, err
		}
		id, err := identity(object["id"], label+".id")
		if err != nil {
			return Metadata{}, err
		}
		if trackIDs[id] {
			return Metadata{}, recordFail("DUPLICATE_IDENTITY", "plan repeats track "+id)
		}
		trackIDs[id] = true
		depends, err := uniqueStringList(object["depends_on"], label+".depends_on", identity)
		if err != nil {
			return Metadata{}, err
		}
		rawSlices, err := asArray(object["slices"], label+".slices", true, MaxListItems)
		if err != nil {
			return Metadata{}, err
		}
		slices := make([]Slice, 0, len(rawSlices))
		for sliceIndex, rawSlice := range rawSlices {
			sliceLabel := fmt.Sprintf("%s.slices[%d]", label, sliceIndex)
			slice, contract, err := validateSlice(rawSlice, id, sliceLabel)
			if err != nil {
				return Metadata{}, err
			}
			if sliceIDs[slice.ID] {
				return Metadata{}, recordFail("DUPLICATE_IDENTITY", "plan repeats slice "+slice.ID)
			}
			sliceIDs[slice.ID] = true
			contracts[slice.ID] = contract
			slices = append(slices, slice)
			totalSlices++
		}
		tracks = append(tracks, Track{ID: id, DependsOn: depends, Slices: slices})
	}
	if totalSlices > MaxSlices {
		return Metadata{}, recordFail("RESOURCE_LIMIT", "plan has too many slices")
	}

	trackEdges := make(map[string][]string, len(tracks))
	for _, track := range tracks {
		for _, dependency := range track.DependsOn {
			if !trackIDs[dependency] || dependency == track.ID {
				return Metadata{}, recordFail("INVALID_DEPENDENCY", "track "+track.ID+" has invalid dependency "+dependency)
			}
		}
		trackEdges[track.ID] = track.DependsOn
	}
	if err := assertAcyclic(keys(trackIDs), trackEdges, "track dependencies"); err != nil {
		return Metadata{}, err
	}

	sliceEdges := make(map[string][]string, totalSlices)
	for _, track := range tracks {
		for _, slice := range track.Slices {
			edges := unique(append(append([]string(nil), slice.DependsOn...), slice.Consumes...))
			for _, dependency := range edges {
				if !sliceIDs[dependency] || dependency == slice.ID {
					return Metadata{}, recordFail("INVALID_DEPENDENCY", "slice "+slice.ID+" has invalid dependency "+dependency)
				}
			}
			sliceEdges[slice.ID] = edges
		}
	}
	if err := assertAcyclic(keys(sliceIDs), sliceEdges, "slice dependencies"); err != nil {
		return Metadata{}, err
	}

	tracksByID := make(map[string]Track, len(tracks))
	deliveryEdges := make(map[string][]string, totalSlices)
	for _, track := range tracks {
		tracksByID[track.ID] = track
		for _, slice := range track.Slices {
			deliveryEdges[slice.ID] = append([]string(nil), sliceEdges[slice.ID]...)
		}
	}
	for _, track := range tracks {
		for index := range track.Slices {
			if index > 0 {
				deliveryEdges[track.Slices[index].ID] = unique(append(
					deliveryEdges[track.Slices[index].ID], track.Slices[index-1].ID,
				))
			}
		}
		first := track.Slices[0].ID
		for _, dependency := range track.DependsOn {
			prior := tracksByID[dependency]
			deliveryEdges[first] = unique(append(deliveryEdges[first], prior.Slices[len(prior.Slices)-1].ID))
		}
	}
	if err := assertAcyclic(keys(sliceIDs), deliveryEdges, "delivery graph"); err != nil {
		return Metadata{}, err
	}
	closures := dependencyClosures(keys(sliceIDs), deliveryEdges)
	for leftIndex := 0; leftIndex < len(tracks); leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(tracks); rightIndex++ {
			left, right := tracks[leftIndex], tracks[rightIndex]
			for _, leftSlice := range left.Slices {
				for _, rightSlice := range right.Slices {
					if closures[leftSlice.ID][rightSlice.ID] || closures[rightSlice.ID][leftSlice.ID] {
						continue
					}
					for _, leftPath := range leftSlice.Scope.Include {
						for _, rightPath := range rightSlice.Scope.Include {
							if pathsOverlap(leftPath, rightPath) {
								return Metadata{}, recordFail(
									"PARALLEL_TOUCH_CONFLICT",
									fmt.Sprintf("independent tracks overlap at %s and %s", leftPath, rightPath),
								)
							}
						}
					}
				}
			}
		}
	}
	return Metadata{
		SchemaVersion: schema, Release: release, Revision: revision, PreviousPlan: previous,
		Repository: repository, TargetRef: targetRef, ApprovalRef: approvalRef,
		Tracks: tracks, Contracts: contracts,
	}, nil
}

type stringValidator func(any, string) (string, error)

func validateSlice(value any, trackID, label string) (Slice, string, error) {
	object, err := asObject(value, label)
	if err != nil {
		return Slice{}, "", err
	}
	required := []string{
		"id", "outcome", "scope", "acceptance", "checks",
		"constraints", "depends_on", "consumes",
	}
	if err := exactKeys(object, required, nil, label); err != nil {
		return Slice{}, "", err
	}
	id, err := identity(object["id"], label+".id")
	if err != nil {
		return Slice{}, "", err
	}
	outcome, err := requiredString(object["outcome"], label+".outcome", 1, 4_096)
	if err != nil {
		return Slice{}, "", err
	}
	scopeObject, err := asObject(object["scope"], label+".scope")
	if err != nil {
		return Slice{}, "", err
	}
	if err := exactKeys(scopeObject, []string{"include", "exclude"}, nil, label+".scope"); err != nil {
		return Slice{}, "", err
	}
	includes, err := uniqueStringList(scopeObject["include"], label+".scope.include", repositoryPath)
	if err != nil {
		return Slice{}, "", err
	}
	if len(includes) == 0 {
		return Slice{}, "", recordFail("INVALID_FIELD", label+".scope.include cannot be empty")
	}
	excludes, err := uniqueStringList(scopeObject["exclude"], label+".scope.exclude", repositoryPath)
	if err != nil {
		return Slice{}, "", err
	}
	rawAcceptance, err := asArray(object["acceptance"], label+".acceptance", true, MaxListItems)
	if err != nil {
		return Slice{}, "", err
	}
	acceptance := make([]Criterion, 0, len(rawAcceptance))
	criterionIDs := make(map[string]bool)
	for index, raw := range rawAcceptance {
		itemLabel := fmt.Sprintf("%s.acceptance[%d]", label, index)
		item, err := asObject(raw, itemLabel)
		if err != nil {
			return Slice{}, "", err
		}
		if err := exactKeys(item, []string{"id", "text"}, nil, itemLabel); err != nil {
			return Slice{}, "", err
		}
		criterionID, err := identity(item["id"], itemLabel+".id")
		if err != nil {
			return Slice{}, "", err
		}
		if criterionIDs[criterionID] {
			return Slice{}, "", recordFail("DUPLICATE_IDENTITY", label+".acceptance repeats "+criterionID)
		}
		criterionIDs[criterionID] = true
		text, err := requiredString(item["text"], itemLabel+".text", 1, 4_096)
		if err != nil {
			return Slice{}, "", err
		}
		acceptance = append(acceptance, Criterion{ID: criterionID, Text: text})
	}
	longString := func(value any, itemLabel string) (string, error) {
		return requiredString(value, itemLabel, 1, 2_048)
	}
	checks, err := uniqueStringList(object["checks"], label+".checks", longString)
	if err != nil {
		return Slice{}, "", err
	}
	constraints, err := uniqueStringList(object["constraints"], label+".constraints", longString)
	if err != nil {
		return Slice{}, "", err
	}
	depends, err := uniqueStringList(object["depends_on"], label+".depends_on", identity)
	if err != nil {
		return Slice{}, "", err
	}
	consumes, err := uniqueStringList(object["consumes"], label+".consumes", identity)
	if err != nil {
		return Slice{}, "", err
	}
	slice := Slice{
		ID: id, Outcome: outcome, Scope: Scope{Include: includes, Exclude: excludes},
		Acceptance: acceptance, Checks: checks, Constraints: constraints,
		DependsOn: depends, Consumes: consumes,
	}
	contractValue := map[string]any{
		"track": trackID,
		"id":    slice.ID, "outcome": slice.Outcome,
		"scope":      map[string]any{"include": slice.Scope.Include, "exclude": slice.Scope.Exclude},
		"acceptance": criteriaAny(slice.Acceptance),
		"checks":     slice.Checks, "constraints": slice.Constraints,
		"depends_on": slice.DependsOn, "consumes": slice.Consumes,
	}
	canonical, err := canonicalJSON(contractValue)
	if err != nil {
		return Slice{}, "", err
	}
	return slice, DigestBytes(canonical), nil
}

func criteriaAny(criteria []Criterion) []any {
	result := make([]any, len(criteria))
	for index, criterion := range criteria {
		result[index] = map[string]any{"id": criterion.ID, "text": criterion.Text}
	}
	return result
}

func identity(value any, label string) (string, error) {
	text, err := requiredString(value, label, 1, 128)
	if err != nil {
		return "", err
	}
	if !identityPattern.MatchString(text) {
		return "", recordFail("INVALID_FIELD", label+" is invalid")
	}
	return text, nil
}

func objectID(value any, label string) (string, error) {
	text, err := requiredString(value, label, 1, 64)
	if err != nil {
		return "", err
	}
	if !objectIDPattern.MatchString(text) {
		return "", recordFail("INVALID_FIELD", label+" is invalid")
	}
	return text, nil
}

func digestString(value any, label string) (string, error) {
	text, err := requiredString(value, label, 1, 71)
	if err != nil {
		return "", err
	}
	if !digestPattern.MatchString(text) {
		return "", recordFail("INVALID_FIELD", label+" is invalid")
	}
	return text, nil
}

func requiredString(value any, label string, min, max int) (string, error) {
	return asString(value, label, min, max)
}

func safeInteger(value any, label string, minimum int64) (int64, error) {
	var result int64
	switch number := value.(type) {
	case json.Number:
		parsed, err := number.Int64()
		if err != nil {
			return 0, recordFail("INVALID_FIELD", fmt.Sprintf("%s must be a safe integer >= %d", label, minimum))
		}
		result = parsed
	case int64:
		result = number
	case int:
		result = int64(number)
	default:
		return 0, recordFail("INVALID_FIELD", fmt.Sprintf("%s must be a safe integer >= %d", label, minimum))
	}
	if result < minimum || result > 9_007_199_254_740_991 {
		return 0, recordFail("INVALID_FIELD", fmt.Sprintf("%s must be a safe integer >= %d", label, minimum))
	}
	return result, nil
}

func headRef(value any, label string) (string, error) {
	ref, err := requiredString(value, label, 1, 250)
	if err != nil {
		return "", err
	}
	if !strings.HasPrefix(ref, "refs/heads/") {
		return "", recordFail("INVALID_FIELD", label+" is invalid")
	}
	tail := strings.TrimPrefix(ref, "refs/heads/")
	if tail == "" || strings.HasPrefix(tail, "/") || strings.Contains(tail, "//") ||
		strings.Contains(tail, "@{") {
		return "", recordFail("INVALID_FIELD", label+" is invalid")
	}
	for _, character := range tail {
		if character < 0x20 || character == 0x7f || strings.ContainsRune(`~^:?*[\`, character) ||
			character == ' ' || character == '\t' || character == '\n' || character == '\r' {
			return "", recordFail("INVALID_FIELD", label+" is invalid")
		}
	}
	for _, part := range strings.Split(tail, "/") {
		if part == "." || part == ".." {
			return "", recordFail("INVALID_FIELD", label+" is invalid")
		}
	}
	return ref, nil
}

func repositoryPath(value any, label string) (string, error) {
	text, err := requiredString(value, label, 1, 512)
	if err != nil {
		return "", err
	}
	if strings.HasPrefix(text, "/") || strings.HasPrefix(text, "\\") ||
		strings.HasSuffix(text, "/") || strings.Contains(text, "//") ||
		strings.Contains(text, "\\") || containsControl(text) {
		return "", recordFail("INVALID_PATH", label+" is not a canonical repository path")
	}
	parts := strings.Split(text, "/")
	for _, part := range parts {
		if part == "" || part == "." || part == ".." {
			return "", recordFail("INVALID_PATH", label+" is not a canonical repository path")
		}
	}
	if parts[0] == ".git" {
		return "", recordFail("INVALID_PATH", label+" is not a canonical repository path")
	}
	return text, nil
}

func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}

func uniqueStringList(value any, label string, validate stringValidator) ([]string, error) {
	items, err := asArray(value, label, false, MaxListItems)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(items))
	seen := make(map[string]bool, len(items))
	for index, raw := range items {
		item, err := validate(raw, fmt.Sprintf("%s[%d]", label, index))
		if err != nil {
			return nil, err
		}
		if seen[item] {
			return nil, recordFail("DUPLICATE_IDENTITY", label+" repeats "+item)
		}
		seen[item] = true
		result = append(result, item)
	}
	return result, nil
}

func pathsOverlap(left, right string) bool {
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func unique(values []string) []string {
	seen := make(map[string]bool, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !seen[value] {
			seen[value] = true
			result = append(result, value)
		}
	}
	return result
}

func keys(values map[string]bool) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func assertAcyclic(nodes []string, edges map[string][]string, label string) error {
	state := make(map[string]uint8, len(nodes))
	var visit func(string) error
	visit = func(node string) error {
		switch state[node] {
		case 1:
			return recordFail("DEPENDENCY_CYCLE", label+" contains a cycle at "+node)
		case 2:
			return nil
		}
		state[node] = 1
		for _, dependency := range edges[node] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		state[node] = 2
		return nil
	}
	for _, node := range nodes {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}

func dependencyClosures(nodes []string, edges map[string][]string) map[string]map[string]bool {
	result := make(map[string]map[string]bool, len(nodes))
	var resolve func(string) map[string]bool
	resolve = func(node string) map[string]bool {
		if value, ok := result[node]; ok {
			return value
		}
		closure := make(map[string]bool)
		// The delivery graph is already proven acyclic, so provisional storage
		// only avoids repeated expansion.
		result[node] = closure
		for _, dependency := range edges[node] {
			closure[dependency] = true
			for inherited := range resolve(dependency) {
				closure[inherited] = true
			}
		}
		return closure
	}
	for _, node := range nodes {
		resolve(node)
	}
	return result
}

func canonicalJSON(value any) ([]byte, error) {
	var result bytes.Buffer
	if err := writeCanonicalJSON(&result, value); err != nil {
		return nil, err
	}
	return result.Bytes(), nil
}

func writeCanonicalJSON(result *bytes.Buffer, value any) error {
	switch typed := value.(type) {
	case nil:
		result.WriteString("null")
	case bool:
		if typed {
			result.WriteString("true")
		} else {
			result.WriteString("false")
		}
	case string:
		writeJSONString(result, typed)
	case json.Number:
		number, err := safeInteger(typed, "canonical value", -9_007_199_254_740_991)
		if err != nil {
			return err
		}
		result.WriteString(fmt.Sprintf("%d", number))
	case int:
		result.WriteString(fmt.Sprintf("%d", typed))
	case int64:
		if typed < -9_007_199_254_740_991 || typed > 9_007_199_254_740_991 {
			return recordFail("UNSAFE_INTEGER", "canonical value is not a safe integer")
		}
		result.WriteString(fmt.Sprintf("%d", typed))
	case []string:
		result.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				result.WriteByte(',')
			}
			writeJSONString(result, item)
		}
		result.WriteByte(']')
	case []any:
		result.WriteByte('[')
		for index, item := range typed {
			if index > 0 {
				result.WriteByte(',')
			}
			if err := writeCanonicalJSON(result, item); err != nil {
				return err
			}
		}
		result.WriteByte(']')
	case map[string]string:
		asAny := make(map[string]any, len(typed))
		for key, item := range typed {
			asAny[key] = item
		}
		return writeCanonicalJSON(result, asAny)
	case map[string]any:
		names := make([]string, 0, len(typed))
		for name := range typed {
			names = append(names, name)
		}
		sort.Strings(names)
		result.WriteByte('{')
		for index, name := range names {
			if index > 0 {
				result.WriteByte(',')
			}
			writeJSONString(result, name)
			result.WriteByte(':')
			if err := writeCanonicalJSON(result, typed[name]); err != nil {
				return err
			}
		}
		result.WriteByte('}')
	default:
		raw, err := json.Marshal(typed)
		if err != nil {
			return recordWrap("INVALID_JSON", "canonicalize value", err)
		}
		parsed, err := strictParseJSON(raw, "canonical value", MaxPlanBytes)
		if err != nil {
			return err
		}
		return writeCanonicalJSON(result, parsed)
	}
	return nil
}

func writeJSONString(result *bytes.Buffer, value string) {
	result.WriteByte('"')
	for _, character := range value {
		switch character {
		case '"', '\\':
			result.WriteByte('\\')
			result.WriteRune(character)
		case '\b':
			result.WriteString(`\b`)
		case '\f':
			result.WriteString(`\f`)
		case '\n':
			result.WriteString(`\n`)
		case '\r':
			result.WriteString(`\r`)
		case '\t':
			result.WriteString(`\t`)
		default:
			if character < 0x20 {
				fmt.Fprintf(result, `\u%04x`, character)
			} else {
				result.WriteRune(character)
			}
		}
	}
	result.WriteByte('"')
}
