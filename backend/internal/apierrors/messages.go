package apierrors

const (
	InvalidJSON      = "Invalid JSON"
	ValidationFailed = "Validation failed"
	MethodNotAllowed = "Method not allowed"
	InternalError    = "Internal server error"
)

const (
	AssetCreateFailed     = "Failed to create asset"
	AssetUpdateInvalidID  = "Invalid Asset ID: %s"
	AssetUpdateInvalidReq = "Invalid Request"
	AssetUpdateFailed     = "Failed to update asset"
	AssetGetInvalidID     = "Invalid Asset ID: %s"
	AssetGetFailed        = "Failed to get asset"
	AssetNotFound         = "Asset not found"
	AssetDeleteInvalidID  = "Invalid Asset ID: %s"
	AssetDeleteFailed     = "Failed to delete asset"
	AssetListFailed       = "Failed to list assets"
	AssetCountFailed      = "Failed to count assets"
)

// Bulk import error messages
const (
	BulkImportJobInvalidID        = "Invalid job ID format"
	BulkImportJobMissingOrg       = "Missing org context"
	BulkImportJobFailedToRetrieve = "Failed to retrieve job"
	BulkImportJobNotFound         = "Job not found or does not belong to your org"
	BulkImportUploadMissingOrg    = "Missing org context"
	BulkImportUploadFailedToParse = "Failed to parse multipart form"
	BulkImportUploadMissingFile   = "Missing or invalid 'file' field"
	BulkImportUploadFailed        = "Upload failed"
)

const (
	AuthSignupInvalidJSON        = "Invalid JSON"
	AuthSignupValidationFailed   = "Validation failed"
	AuthSignupEmailExists        = "Email already exists"
	AuthSignupOrgIdentifierTaken = "Organization identifier already taken"
	AuthSignupFailed             = "Failed to signup"
	AuthLoginInvalidJSON         = "Invalid JSON"
	AuthLoginValidationFailed    = "Validation failed"
	AuthLoginInvalidCredentials  = "Invalid email or password"
	AuthLoginFailed              = "Failed to login"
)

// Password reset error messages
const (
	AuthForgotPasswordInvalidJSON = "Invalid JSON"
	AuthForgotPasswordValidation  = "Validation failed"
	AuthForgotPasswordFailed      = "Failed to process request"
	AuthResetPasswordInvalidJSON  = "Invalid JSON"
	AuthResetPasswordValidation   = "Validation failed"
	AuthResetPasswordInvalidToken = "Invalid or expired reset link"
	AuthResetPasswordFailed       = "Failed to reset password"
)

const (
	UserListFailed           = "Failed to list users"
	UserGetInvalidID         = "Invalid user ID"
	UserGetFailed            = "Failed to get user"
	UserNotFound             = "User not found"
	UserCreateInvalidJSON    = "Invalid JSON"
	UserCreateValidationFail = "Validation failed"
	UserCreateEmailExists    = "Email already exists"
	UserCreateFailed         = "Failed to create user"
	UserUpdateInvalidID      = "Invalid user ID"
	UserUpdateInvalidJSON    = "Invalid JSON"
	UserUpdateValidationFail = "Validation failed"
	UserUpdateEmailExists    = "Email already exists"
	UserUpdateFailed         = "Failed to update user"
	UserUpdateNotFound       = "User not found"
	UserDeleteInvalidID      = "Invalid user ID"
	UserDeleteNotFound       = "User not found"
	UserDeleteFailed         = "Failed to delete user"
)

const (
	LocationCreateFailed     = "Failed to create location"
	LocationUpdateInvalidID  = "Invalid Location ID: %s"
	LocationUpdateInvalidReq = "Invalid Request"
	LocationUpdateFailed     = "Failed to update location"
	LocationGetInvalidID     = "Invalid Location ID: %s"
	LocationGetFailed        = "Failed to get location"
	LocationNotFound         = "Location not found"
	LocationDeleteInvalidID  = "Invalid Location ID: %s"
	LocationDeleteFailed     = "Failed to delete location"
	LocationListFailed       = "Failed to list locations"
	LocationCountFailed      = "Failed to count locations"
)

// Identifier (tag) error messages
const (
	IdentifierDuplicateValue = "Identifier with this type and value already exists"
	IdentifierInvalidType    = "Identifier type must be rfid, ble, or barcode"
	IdentifierCreateFailed   = "Failed to create identifier"
	IdentifierNotFound       = "Identifier not found"
	IdentifierDeleteFailed   = "Failed to delete identifier"
	IdentifierInvalidID      = "Invalid identifier ID: %s"
)

// Organization error messages
const (
	OrgListFailed           = "Failed to list organizations"
	OrgGetInvalidID         = "Invalid organization ID"
	OrgGetFailed            = "Failed to get organization"
	OrgNotFound             = "Organization not found"
	OrgCreateInvalidJSON    = "Invalid JSON"
	OrgCreateValidationFail = "Validation failed"
	OrgCreateFailed         = "Failed to create organization"
	OrgUpdateInvalidID      = "Invalid organization ID"
	OrgUpdateInvalidJSON    = "Invalid JSON"
	OrgUpdateValidationFail = "Validation failed"
	OrgUpdateFailed         = "Failed to update organization"
	OrgUpdateNotFound       = "Organization not found"
	OrgDeleteInvalidID      = "Invalid organization ID"
	OrgDeleteInvalidJSON    = "Invalid JSON"
	OrgDeleteNameMismatch   = "Organization name does not match"
	OrgDeleteFailed         = "Failed to delete organization"
	OrgDeleteNotFound       = "Organization not found"
	OrgNotMember            = "You are not a member of this organization"
	OrgSetCurrentFailed     = "Failed to set current organization"
)

// Member management error messages
const (
	MemberListFailed           = "Failed to list members"
	MemberUpdateInvalidID      = "Invalid user ID"
	MemberUpdateInvalidJSON    = "Invalid JSON"
	MemberUpdateValidationFail = "Validation failed"
	MemberUpdateFailed         = "Failed to update member role"
	MemberNotFound             = "Member not found"
	MemberRemoveFailed         = "Failed to remove member"
	MemberLastAdmin            = "Cannot remove or demote the last admin"
	MemberSelfRemoval          = "Cannot remove yourself"
	MemberInvalidRole          = "Invalid role"
)

// Invitation error messages
const (
	InvitationListFailed          = "Failed to list invitations"
	InvitationCreateInvalidJSON   = "Invalid JSON"
	InvitationCreateValidation    = "Validation failed"
	InvitationCreateFailed        = "Failed to create invitation"
	InvitationAlreadyMember       = "%s is already a member of this organization"
	InvitationAlreadyPending      = "An invitation is already pending for %s"
	InvitationNotFound            = "Invitation not found"
	InvitationCancelFailed        = "Failed to cancel invitation"
	InvitationResendFailed        = "Failed to resend invitation"
	InvitationInvalidID           = "Invalid invitation ID"
	InvitationExpired             = "This invitation has expired"
	InvitationCancelled           = "This invitation has been cancelled"
	InvitationAcceptInvalidJSON   = "Invalid JSON"
	InvitationAcceptValidation    = "Validation failed"
	InvitationAcceptFailed        = "Failed to accept invitation"
	InvitationAcceptAlreadyMember = "You are already a member of this organization"
	InvitationAcceptAlreadyUsed   = "This invitation has already been accepted"
	InvitationAcceptEmailMismatch = "This invitation was sent to %s"
	InvitationInvalidToken        = "Invalid invitation token"
)
