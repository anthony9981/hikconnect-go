module hikvision-bridge

go 1.25

require github.com/elyor04/go-hikvision-sdk v1.2.3

// The wrapper requires HCNetSDK headers/libs vendored inside the module
// (internal/sdklib). Docker clones the repo at /hiksdk and vendors the SDK
// there before building - see Dockerfile.
replace github.com/elyor04/go-hikvision-sdk => /hiksdk
