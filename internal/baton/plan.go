package baton

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"slices"
	"strings"
)

const (
	planOpen  = "```baton-plan-v1\n"
	planClose = "\n```\n"
)

var (
	identityPattern    = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$`)
	digestPattern      = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)
	objectIDPattern    = regexp.MustCompile(`^(?:[0-9a-f]{40}|[0-9a-f]{64})$`)
	invocationPattern  = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._:/-]{0,199}$`)
	blockerCodePattern = regexp.MustCompile(`^[a-z][a-z0-9_]{0,63}$`)
)

type Criterion struct{ ID, Text string }
type WorkScope struct{ Include, Exclude []string }
type Work struct {
	ID          string
	Outcome     string
	Scope       WorkScope
	Acceptance  []Criterion
	Checks      []string
	Constraints []string
	DependsOn   []string
}
type Track struct {
	ID            string
	Ref           string
	DependsOn     []string
	TouchSurfaces []string
	Work          []Work
}
type Metadata struct {
	SchemaVersion string
	Release       string
	Repository    string
	TargetRef     string
	ReleaseRef    string
	RecordRoot    string
	ApprovalRef   string
	Tracks        []Track
}
type planAdmission struct {
	raw      []byte
	digest   string
	metadata Metadata
	markdown string
}
type Plan struct {
	admission *planAdmission
}

func ParsePlan(raw []byte) (Plan, error) {
	if len(raw) > MaxPlanBytes {
		return Plan{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("plan.md exceeds maximum size %d bytes", MaxPlanBytes))
	}
	text := string(raw)
	if !strings.HasPrefix(text, planOpen) {
		return Plan{}, recordFail("INVALID_PLAN_FENCE", "plan.md must begin at byte zero with ```baton-plan-v1")
	}
	closeIndex := strings.Index(text[len(planOpen):], planClose)
	if closeIndex < 0 {
		return Plan{}, recordFail("INVALID_PLAN_FENCE", "plan.md is missing the exact closing fence")
	}
	closeIndex += len(planOpen)
	metadataBytes := []byte(text[len(planOpen):closeIndex])
	if len(metadataBytes) == 0 {
		return Plan{}, recordFail("INVALID_PLAN_METADATA", "plan metadata is empty")
	}
	value, err := strictParseJSON(metadataBytes, "plan metadata", MaxPlanBytes)
	if err != nil {
		return Plan{}, err
	}
	object, err := asObject(value, "plan")
	if err != nil {
		return Plan{}, err
	}
	metadata, err := validatePlanMetadata(object)
	if err != nil {
		return Plan{}, err
	}
	copied := append([]byte(nil), raw...)
	return Plan{admission: &planAdmission{
		raw:      copied,
		digest:   DigestBytes(copied),
		metadata: metadata,
		markdown: text[closeIndex+len(planClose):],
	}}, nil
}
func DigestBytes(raw []byte) string {
	sum := sha256.Sum256(raw)
	return "sha256:" + hex.EncodeToString(sum[:])
}
func (p Plan) require() (*planAdmission, error) {
	if p.admission == nil || !digestPattern.MatchString(p.admission.digest) ||
		DigestBytes(p.admission.raw) != p.admission.digest {
		return nil, recordFail("PLAN_ADMISSION_REQUIRED", "operation requires the immutable parsed plan bound to its exact raw digest")
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
	result.Tracks = make([]Track, len(value.Tracks))
	for index, track := range value.Tracks {
		result.Tracks[index] = track
		result.Tracks[index].DependsOn = append([]string(nil), track.DependsOn...)
		result.Tracks[index].TouchSurfaces = append([]string(nil), track.TouchSurfaces...)
		result.Tracks[index].Work = make([]Work, len(track.Work))
		for workIndex, work := range track.Work {
			result.Tracks[index].Work[workIndex] = work
			result.Tracks[index].Work[workIndex].Scope.Include = append([]string(nil), work.Scope.Include...)
			result.Tracks[index].Work[workIndex].Scope.Exclude = append([]string(nil), work.Scope.Exclude...)
			result.Tracks[index].Work[workIndex].Acceptance = append([]Criterion(nil), work.Acceptance...)
			result.Tracks[index].Work[workIndex].Checks = append([]string(nil), work.Checks...)
			result.Tracks[index].Work[workIndex].Constraints = append([]string(nil), work.Constraints...)
			result.Tracks[index].Work[workIndex].DependsOn = append([]string(nil), work.DependsOn...)
		}
	}
	return result
}
func validatePlanMetadata(value map[string]any) (Metadata, error) {
	required := []string{"schema_version", "release", "repository", "target_ref", "release_ref", "record_root", "approval_ref", "tracks"}
	if err := exactKeys(value, required, nil, "plan"); err != nil {
		return Metadata{}, err
	}
	schema, err := asString(value["schema_version"], "plan.schema_version", 1, 100)
	if err != nil {
		return Metadata{}, err
	}
	if schema != "baton.plan/v1" {
		return Metadata{}, recordFail("INVALID_VERSION", "plan.schema_version must be baton.plan/v1")
	}
	release, err := identity(value["release"], "plan.release")
	if err != nil {
		return Metadata{}, err
	}
	repository, err := asString(value["repository"], "plan.repository", 1, 500)
	if err != nil || containsControl(repository) {
		return Metadata{}, recordFail("INVALID_FIELD", "plan.repository contains an invalid value")
	}
	targetRef, err := headRef(value["target_ref"], "plan.target_ref")
	if err != nil {
		return Metadata{}, err
	}
	releaseRef, err := headRef(value["release_ref"], "plan.release_ref")
	if err != nil {
		return Metadata{}, err
	}
	if releaseRef != "refs/heads/release-wt/"+release {
		return Metadata{}, recordFail("INVALID_REF", "plan.release_ref must be refs/heads/release-wt/"+release)
	}
	if targetRef == releaseRef {
		return Metadata{}, recordFail("INVALID_REF", "plan target and release refs must differ")
	}
	recordRoot, err := repositoryPath(value["record_root"], "plan.record_root", false)
	if err != nil {
		return Metadata{}, err
	}
	if recordRoot != ".baton/releases" {
		return Metadata{}, recordFail("INVALID_RECORD_ROOT", "plan.record_root must be exactly .baton/releases in v1")
	}
	approvalRef, err := artifactRef(value["approval_ref"], "plan.approval_ref")
	if err != nil {
		return Metadata{}, err
	}
	trackValues, err := asArray(value["tracks"], "plan.tracks", true, MaxTracks)
	if err != nil {
		return Metadata{}, err
	}
	tracks := make([]Track, 0, len(trackValues))
	trackIDs := make(map[string]struct{}, len(trackValues))
	workIDs := make(map[string]struct{})
	workTrack := make(map[string]string)
	workOrder := make(map[string]int)
	for trackIndex, rawTrack := range trackValues {
		label := fmt.Sprintf("plan.tracks[%d]", trackIndex)
		trackObject, err := asObject(rawTrack, label)
		if err != nil {
			return Metadata{}, err
		}
		if err := exactKeys(trackObject, []string{"id", "ref", "depends_on", "touch_surfaces", "work"}, nil, label); err != nil {
			return Metadata{}, err
		}
		id, err := identity(trackObject["id"], label+".id")
		if err != nil {
			return Metadata{}, err
		}
		if _, duplicate := trackIDs[id]; duplicate {
			return Metadata{}, recordFail("DUPLICATE_IDENTITY", "plan repeats track "+id)
		}
		trackIDs[id] = struct{}{}
		ref, err := headRef(trackObject["ref"], label+".ref")
		if err != nil {
			return Metadata{}, err
		}
		if ref != "refs/heads/track/"+release+"/"+id {
			return Metadata{}, recordFail("INVALID_REF", label+".ref must be refs/heads/track/"+release+"/"+id)
		}
		depends, err := stringList(trackObject["depends_on"], label+".depends_on", false, false)
		if err != nil {
			return Metadata{}, err
		}
		for index := range depends {
			if !identityPattern.MatchString(depends[index]) {
				return Metadata{}, recordFail("INVALID_FIELD", label+".depends_on contains an invalid identity")
			}
		}
		touches, err := stringList(trackObject["touch_surfaces"], label+".touch_surfaces", true, true)
		if err != nil {
			return Metadata{}, err
		}
		for _, touch := range touches {
			if pathsOverlap(touch, recordRoot) {
				return Metadata{}, recordFail("RECORD_ROOT_IN_PRODUCT_SCOPE", label+" touch surface overlaps record root "+recordRoot)
			}
		}
		workValues, err := asArray(trackObject["work"], label+".work", true, MaxWorkPerTrack)
		if err != nil {
			return Metadata{}, err
		}
		work := make([]Work, 0, len(workValues))
		for workIndex, rawWork := range workValues {
			parsed, err := validatePlanWork(rawWork, fmt.Sprintf("%s.work[%d]", label, workIndex), recordRoot)
			if err != nil {
				return Metadata{}, err
			}
			for _, scoped := range append(append([]string(nil), parsed.Scope.Include...), parsed.Scope.Exclude...) {
				inside := false
				for _, touch := range touches {
					if pathContains(touch, scoped) {
						inside = true
						break
					}
				}
				if !inside {
					return Metadata{}, recordFail("WORK_OUTSIDE_TRACK_SCOPE", fmt.Sprintf("%s work %s scope %s is outside track %s touch surfaces", label, parsed.ID, scoped, id))
				}
			}
			for _, excluded := range parsed.Scope.Exclude {
				valid := false
				for _, included := range parsed.Scope.Include {
					if pathContains(included, excluded) {
						valid = true
					}
				}
				if !valid {
					return Metadata{}, recordFail("INVALID_WORK_SCOPE", "exclude is not inside an included path")
				}
			}
			if _, duplicate := workIDs[parsed.ID]; duplicate {
				return Metadata{}, recordFail("DUPLICATE_IDENTITY", "plan repeats work "+parsed.ID)
			}
			workIDs[parsed.ID] = struct{}{}
			workTrack[parsed.ID] = id
			workOrder[parsed.ID] = workIndex
			work = append(work, parsed)
		}
		tracks = append(tracks, Track{ID: id, Ref: ref, DependsOn: depends, TouchSurfaces: touches, Work: work})
	}
	if len(workIDs) > MaxWorkTotal {
		return Metadata{}, recordFail("RESOURCE_LIMIT", fmt.Sprintf("plan exceeds maximum total work %d", MaxWorkTotal))
	}
	trackEdges := make(map[string][]string, len(tracks))
	for _, track := range tracks {
		trackEdges[track.ID] = track.DependsOn
		for _, dependency := range track.DependsOn {
			if _, exists := trackIDs[dependency]; !exists {
				return Metadata{}, recordFail("DANGLING_DEPENDENCY", "track "+track.ID+" depends on unknown "+dependency)
			}
			if dependency == track.ID {
				return Metadata{}, recordFail("DEPENDENCY_CYCLE", "track "+track.ID+" depends on itself")
			}
		}
	}
	if err := detectCycle(mapKeys(trackIDs), trackEdges, "track graph"); err != nil {
		return Metadata{}, err
	}
	workEdges := make(map[string][]string, len(workIDs))
	for _, track := range tracks {
		closure := dependencyClosure(track.ID, trackEdges, nil)
		for _, work := range track.Work {
			workEdges[work.ID] = work.DependsOn
			for _, dependency := range work.DependsOn {
				dependencyTrack, exists := workTrack[dependency]
				if !exists {
					return Metadata{}, recordFail("DANGLING_DEPENDENCY", "work "+work.ID+" depends on unknown "+dependency)
				}
				if dependencyTrack == track.ID && workOrder[dependency] >= workOrder[work.ID] {
					return Metadata{}, recordFail("OUT_OF_ORDER_DEPENDENCY", "work "+work.ID+" depends on later work "+dependency)
				}
				if dependencyTrack != track.ID && !closure[dependencyTrack] {
					return Metadata{}, recordFail("UNDECLARED_TRACK_DEPENDENCY", "work dependency has no track dependency")
				}
			}
		}
	}
	if err := detectCycle(mapKeys(workIDs), workEdges, "work graph"); err != nil {
		return Metadata{}, err
	}
	for leftIndex := 0; leftIndex < len(tracks); leftIndex++ {
		for rightIndex := leftIndex + 1; rightIndex < len(tracks); rightIndex++ {
			left, right := tracks[leftIndex], tracks[rightIndex]
			related := dependencyClosure(left.ID, trackEdges, nil)[right.ID] ||
				dependencyClosure(right.ID, trackEdges, nil)[left.ID]
			if related {
				continue
			}
			for _, leftWork := range left.Work {
				for _, rightWork := range right.Work {
					for _, leftPath := range leftWork.Scope.Include {
						for _, rightPath := range rightWork.Scope.Include {
							if pathsOverlap(leftPath, rightPath) {
								return Metadata{}, recordFail("PARALLEL_WORK_SCOPE_CONFLICT", "independent work scopes overlap")
							}
						}
					}
				}
			}
			for _, leftPath := range left.TouchSurfaces {
				for _, rightPath := range right.TouchSurfaces {
					if pathsOverlap(leftPath, rightPath) {
						return Metadata{}, recordFail("PARALLEL_TOUCH_CONFLICT", "independent track touch surfaces overlap")
					}
				}
			}
		}
	}
	return Metadata{
		SchemaVersion: schema,
		Release:       release,
		Repository:    repository,
		TargetRef:     targetRef,
		ReleaseRef:    releaseRef,
		RecordRoot:    recordRoot,
		ApprovalRef:   approvalRef,
		Tracks:        tracks,
	}, nil
}
func validatePlanWork(value any, label, recordRoot string) (Work, error) {
	object, err := asObject(value, label)
	if err != nil {
		return Work{}, err
	}
	if err := exactKeys(object, []string{"id", "outcome", "scope", "acceptance", "checks", "constraints", "depends_on"}, nil, label); err != nil {
		return Work{}, err
	}
	id, err := identity(object["id"], label+".id")
	if err != nil {
		return Work{}, err
	}
	outcome, err := asString(object["outcome"], label+".outcome", 1, 1000)
	if err != nil {
		return Work{}, err
	}
	scopeObject, err := asObject(object["scope"], label+".scope")
	if err != nil {
		return Work{}, err
	}
	if err := exactKeys(scopeObject, []string{"include", "exclude"}, nil, label+".scope"); err != nil {
		return Work{}, err
	}
	include, err := stringList(scopeObject["include"], label+".scope.include", true, true)
	if err != nil {
		return Work{}, err
	}
	exclude, err := stringList(scopeObject["exclude"], label+".scope.exclude", false, true)
	if err != nil {
		return Work{}, err
	}
	for _, scoped := range append(append([]string(nil), include...), exclude...) {
		if pathContains(recordRoot, scoped) || pathContains(scoped, recordRoot) {
			return Work{}, recordFail("RECORD_ROOT_IN_PRODUCT_SCOPE", label+" scope overlaps record root "+recordRoot)
		}
	}
	acceptanceValues, err := asArray(object["acceptance"], label+".acceptance", true, MaxListItems)
	if err != nil {
		return Work{}, err
	}
	acceptance := make([]Criterion, 0, len(acceptanceValues))
	seenAcceptance := make(map[string]struct{})
	for index, rawCriterion := range acceptanceValues {
		criterionLabel := fmt.Sprintf("%s.acceptance[%d]", label, index)
		criterionObject, err := asObject(rawCriterion, criterionLabel)
		if err != nil {
			return Work{}, err
		}
		if err := exactKeys(criterionObject, []string{"id", "text"}, nil, criterionLabel); err != nil {
			return Work{}, err
		}
		criterionID, err := identity(criterionObject["id"], criterionLabel+".id")
		if err != nil {
			return Work{}, err
		}
		if _, duplicate := seenAcceptance[criterionID]; duplicate {
			return Work{}, recordFail("DUPLICATE_IDENTITY", label+" repeats acceptance "+criterionID)
		}
		seenAcceptance[criterionID] = struct{}{}
		text, err := asString(criterionObject["text"], criterionLabel+".text", 1, 2000)
		if err != nil {
			return Work{}, err
		}
		acceptance = append(acceptance, Criterion{ID: criterionID, Text: text})
	}
	checks, err := stringList(object["checks"], label+".checks", false, false)
	if err != nil {
		return Work{}, err
	}
	constraints, err := stringList(object["constraints"], label+".constraints", false, false)
	if err != nil {
		return Work{}, err
	}
	depends, err := stringList(object["depends_on"], label+".depends_on", false, false)
	if err != nil {
		return Work{}, err
	}
	for _, dependency := range depends {
		if !identityPattern.MatchString(dependency) {
			return Work{}, recordFail("INVALID_FIELD", label+".depends_on contains an invalid identity")
		}
	}
	return Work{
		ID: id, Outcome: outcome, Scope: WorkScope{Include: include, Exclude: exclude},
		Acceptance: acceptance, Checks: checks, Constraints: constraints, DependsOn: depends,
	}, nil
}
func identity(value any, label string) (string, error) {
	result, err := asString(value, label, 1, 128)
	if err != nil || !identityPattern.MatchString(result) {
		return "", recordFail("INVALID_FIELD", label+" has an invalid value")
	}
	return result, nil
}
func digestValue(value any, label string) (string, error) {
	result, err := asString(value, label, 1, 71)
	if err != nil || !digestPattern.MatchString(result) {
		return "", recordFail("INVALID_FIELD", label+" has an invalid value")
	}
	return result, nil
}
func objectIDValue(value any, label string) (string, error) {
	result, err := asString(value, label, 1, 64)
	if err != nil || !objectIDPattern.MatchString(result) {
		return "", recordFail("INVALID_FIELD", label+" has an invalid value")
	}
	return result, nil
}
func invocationValue(value any, label string) (string, error) {
	result, err := asString(value, label, 1, 200)
	if err != nil || !invocationPattern.MatchString(result) {
		return "", recordFail("INVALID_FIELD", label+" has an invalid value")
	}
	return result, nil
}
func artifactRef(value any, label string) (string, error) {
	result, err := asString(value, label, 1, 1000)
	if err != nil || containsControl(result) {
		return "", recordFail("INVALID_FIELD", label+" contains an invalid value")
	}
	return result, nil
}
func headRef(value any, label string) (string, error) {
	result, err := asString(value, label, 1, 250)
	if err != nil || !strings.HasPrefix(result, "refs/heads/") {
		return "", recordFail("INVALID_REF", label+" must be a full refs/heads ref")
	}
	tail := strings.TrimPrefix(result, "refs/heads/")
	for _, segment := range strings.Split(tail, "/") {
		if segment == "" || segment == "." || segment == ".." || strings.HasPrefix(segment, ".") ||
			strings.HasSuffix(segment, ".") || strings.HasSuffix(segment, ".lock") {
			return "", recordFail("INVALID_REF", label+" is not a canonical branch ref")
		}
	}
	if strings.Contains(tail, "..") || strings.Contains(tail, "@{") ||
		strings.ContainsAny(tail, `\ ~^:?*[]`) || containsControl(tail) {
		return "", recordFail("INVALID_REF", label+" is not a canonical branch ref")
	}
	return result, nil
}
func repositoryPath(value any, label string, allowRoot bool) (string, error) {
	result, err := asString(value, label, 1, 1000)
	if err != nil {
		return "", err
	}
	if allowRoot && result == "." {
		return result, nil
	}
	if path.IsAbs(result) || strings.Contains(result, `\`) || containsControl(result) {
		return "", recordFail("INVALID_PATH", label+" must be repository-relative")
	}
	segments := strings.Split(result, "/")
	if segments[0] == ".git" {
		return "", recordFail("INVALID_PATH", label+" is not canonical")
	}
	for _, segment := range segments {
		if segment == "" || segment == "." || segment == ".." {
			return "", recordFail("INVALID_PATH", label+" is not canonical")
		}
	}
	return result, nil
}
func stringList(value any, label string, nonempty, paths bool) ([]string, error) {
	values, err := asArray(value, label, nonempty, MaxListItems)
	if err != nil {
		return nil, err
	}
	result := make([]string, 0, len(values))
	seen := make(map[string]struct{})
	for index, raw := range values {
		var parsed string
		if paths {
			parsed, err = repositoryPath(raw, fmt.Sprintf("%s[%d]", label, index), true)
		} else {
			parsed, err = asString(raw, fmt.Sprintf("%s[%d]", label, index), 1, 1000)
		}
		if err != nil {
			return nil, err
		}
		if _, duplicate := seen[parsed]; duplicate {
			return nil, recordFail("DUPLICATE_IDENTITY", label+" repeats "+parsed)
		}
		seen[parsed] = struct{}{}
		result = append(result, parsed)
	}
	return result, nil
}
func pathContains(parent, child string) bool {
	return parent == "." || child == parent || strings.HasPrefix(child, parent+"/")
}
func pathsOverlap(left, right string) bool {
	return pathContains(left, right) || pathContains(right, left)
}
func containsControl(value string) bool {
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return true
		}
	}
	return false
}
func mapKeys(values map[string]struct{}) []string {
	result := make([]string, 0, len(values))
	for value := range values {
		result = append(result, value)
	}
	slices.Sort(result)
	return result
}
func detectCycle(nodes []string, edges map[string][]string, label string) error {
	visiting := make(map[string]bool)
	visited := make(map[string]bool)
	var visit func(string) error
	visit = func(node string) error {
		if visiting[node] {
			return recordFail("DEPENDENCY_CYCLE", label+" contains a cycle through "+node)
		}
		if visited[node] {
			return nil
		}
		visiting[node] = true
		for _, dependency := range edges[node] {
			if err := visit(dependency); err != nil {
				return err
			}
		}
		delete(visiting, node)
		visited[node] = true
		return nil
	}
	for _, node := range nodes {
		if err := visit(node); err != nil {
			return err
		}
	}
	return nil
}
func dependencyClosure(track string, edges map[string][]string, memo map[string]map[string]bool) map[string]bool {
	if memo == nil {
		memo = make(map[string]map[string]bool)
	}
	if result := memo[track]; result != nil {
		return result
	}
	result := make(map[string]bool)
	memo[track] = result
	for _, dependency := range edges[track] {
		result[dependency] = true
		for nested := range dependencyClosure(dependency, edges, memo) {
			result[nested] = true
		}
	}
	return result
}
func (p Plan) FindTrack(trackID string) (Track, bool) {
	metadata := p.Metadata()
	for _, track := range metadata.Tracks {
		if track.ID == trackID {
			return track, true
		}
	}
	return Track{}, false
}
func (p Plan) FindWork(workID string) (Track, Work, bool) {
	metadata := p.Metadata()
	for _, track := range metadata.Tracks {
		for _, work := range track.Work {
			if work.ID == workID {
				return track, work, true
			}
		}
	}
	return Track{}, Work{}, false
}
func releasePath(plan Plan) string {
	metadata := plan.Metadata()
	return metadata.RecordRoot + "/" + metadata.Release
}
func WorkStatusPath(plan Plan, workID string) string {
	return releasePath(plan) + "/work/" + workID + "/status.json"
}
func WorkDesignPath(plan Plan, workID string) string {
	return releasePath(plan) + "/work/" + workID + "/design.md"
}
func WorkProofPath(plan Plan, workID string) string {
	return releasePath(plan) + "/work/" + workID + "/proof.md"
}
func AssemblyStatusPath(plan Plan) string { return releasePath(plan) + "/assembly/status.json" }
func AssemblyProofPath(plan Plan) string  { return releasePath(plan) + "/assembly/proof.md" }
func ReleasePlanPath(plan Plan) string    { return releasePath(plan) + "/plan.md" }
