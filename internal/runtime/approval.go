package runtime

import (
	"fmt"
	"strconv"

	"github.com/swornagent/sworn/internal/baton"
)

// approvalAdmission is local, exact authority for one plan. It deliberately
// contains no hosted identity, credential, URL, issue or external evidence.
type approvalAdmission struct {
	planBytes  []byte
	planDigest string
	reference  string
}

func validateApprovalRef(manifest admittedManifest, plan baton.Plan) error {
	metadata := plan.Metadata()
	expected := manifest.value.Authority.ExternalAuthorizer + "://" +
		manifest.value.Release + "/" + strconv.FormatInt(metadata.Revision, 10)
	if metadata.ApprovalRef != expected {
		return runtimeFail("APPROVAL_BINDING_MISMATCH", nil)
	}
	return nil
}

type authorityInstaller struct {
	actions *baton.Actions
}

func newAuthorityInstaller(actions *baton.Actions) *authorityInstaller {
	return &authorityInstaller{actions: actions}
}

func installDetail(admission approvalAdmission) []byte {
	return []byte(fmt.Sprintf(
		"Sworn project authority admitted %s via %s.\n",
		admission.planDigest,
		admission.reference,
	))
}

// install is the only runtime path authorized to call RecordPlanRevision.
func (i *authorityInstaller) install(admission approvalAdmission) (baton.ActionResult, error) {
	if i == nil || i.actions == nil ||
		!runtimeDigestPattern.MatchString(admission.planDigest) ||
		sha256Digest(admission.planBytes) != admission.planDigest ||
		admission.reference == "" {
		return baton.ActionResult{}, runtimeFail("APPROVAL_ADMISSION_REQUIRED", nil)
	}
	return i.actions.RecordPlanRevision(baton.RecordPlanRevisionInput{
		PlanBytes: admission.planBytes,
		Summary:   "Install the exact locally authorized plan.",
		Detail:    installDetail(admission),
	})
}
