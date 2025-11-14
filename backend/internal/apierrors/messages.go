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
