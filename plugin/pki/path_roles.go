package pki

import (
	"context"
	"fmt"
	"time"

	"github.com/hashicorp/vault/logical"
	"github.com/hashicorp/vault/logical/framework"
)

func pathListRoles(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "roles/?$",

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ListOperation: b.pathRoleList,
		},

		HelpSynopsis:    pathListRolesHelpSyn,
		HelpDescription: pathListRolesHelpDesc,
	}
}

func pathRoles(b *backend) *framework.Path {
	return &framework.Path{
		Pattern: "roles/" + framework.GenericNameRegex("name"),
		Fields: map[string]*framework.FieldSchema{
			"name": {
				Type:        framework.TypeString,
				Description: "Name of the role",
			},
			"tpp_url": {
				Type:        framework.TypeString,
				Description: `URL of Venafi Platfrom. Example: https://tpp.venafi.example/vedsdk`,
			},

			"cloud_url": {
				Type:        framework.TypeString,
				Description: `URL for Venafi Cloud. Set it only if you want to use non production Cloud`,
			},

			"zone": {
				Type: framework.TypeString,
				Description: `Name of Venafi Platfrom or Cloud policy. 
Example for Platform: testpolicy\\vault
Example for Venafi Cloud: e33f3e40-4e7e-11ea-8da3-b3c196ebeb0b`,
				Required: true,
			},

			"tpp_user": {
				Type:        framework.TypeString,
				Description: `web API user for Venafi Platfrom Example: admin`,
			},
			"tpp_password": {
				Type:        framework.TypeString,
				Description: `Password for web API user Example: password`,
			},
			"trust_bundle_file": {
				Type: framework.TypeString,
				Description: `Use to specify a PEM formatted file with certificates to be used as trust anchors when communicating with the remote server.
Example:
  trust_bundle_file = "/full/path/to/bundle.pem""`,
			},
			"apikey": {
				Type:        framework.TypeString,
				Description: `API key for Venafi Cloud. Example: 142231b7-cvb0-412e-886b-6aeght0bc93d`,
			},
			"fakemode": {
				Type:        framework.TypeBool,
				Description: `Set it to true to use face CA instead of Cloud or Platform to issue certificates. Useful for testing.`,
				Default:     false,
			},

			"store_by_cn": {
				Type:        framework.TypeBool,
				Description: `Set it to true to store certificates by CN in certs/ path`,
				Deprecated:  true,
			},

			"store_by_serial": {
				Type:        framework.TypeBool,
				Description: `Set it to true to store certificates by unique serial number in certs/ path`,
				Deprecated:  true,
			},

			"store_by": {
				Type: framework.TypeString,
				Description: `The attribute by which certificates are stored in the backend.  "serial" (default) and "cn" are the only valid values.`,
			},

			"no_store": {
				Type:        framework.TypeBool,
				Description: `If set, certificates issued/signed against this role will not be stored in the storage backend.`,
			},

			"service_generated_cert": {
				Type:        framework.TypeBool,
				Description: `Use service generated CSR for Venafi Platfrom (ignored if Saas endpoint used)`,
				Default:     false,
			},
			"store_pkey": {
				Type:        framework.TypeBool,
				Description: `Set it to true to store certificates privates key in certificate fields`,
			},
			"chain_option": {
				Type:        framework.TypeString,
				Description: `Specify ordering certificates in chain. Root can be "first" or "last"`,
				Default:     "last",
			},
			"key_type": {
				Type:    framework.TypeString,
				Default: "rsa",
				Description: `The type of key to use; defaults to RSA. "rsa"
				and "ec" (ECDSA) are the only valid values.`,
			},
			"key_bits": {
				Type:    framework.TypeInt,
				Default: 2048,
				Description: `The number of bits to use. You will almost
certainly want to change this if you adjust
the key_type. Default: 2048`,
			},
			"key_curve": {
				Type:        framework.TypeString,
				Default:     "P256",
				Description: `Key curve for EC key type. Valid values are: "P256","P384","P521"`,
			},
			"ttl": {
				Type: framework.TypeDurationSecond,
				Description: `The lease duration if no specific lease duration is
requested. The lease duration controls the expiration
of certificates issued by this backend. Defaults to
the value of max_ttl.`,
			},

			"max_ttl": {
				Type:        framework.TypeDurationSecond,
				Description: "The maximum allowed lease duration",
			},

			"generate_lease": {
				Type: framework.TypeBool,
				Description: `
If set, certificates issued/signed against this role will have Vault leases
attached to them. Defaults to "false".`,
			},
			"server_timeout": {
				Type:        framework.TypeInt,
				Description: "Timeout of waiting certificate",
				Default:     180,
			},
		},

		Callbacks: map[logical.Operation]framework.OperationFunc{
			logical.ReadOperation:   b.pathRoleRead,
			logical.UpdateOperation: b.pathRoleCreate,
			logical.DeleteOperation: b.pathRoleDelete,
		},

		HelpSynopsis:    pathRoleHelpSyn,
		HelpDescription: pathRoleHelpDesc,
	}
}

const (
	storeByCNString                              = "cn"
	storeBySerialString                          = "serial"
	errorTextInvalidMode                         = "Invalid mode. fakemode or apikey or tpp credentials required"
	errorTextValueMustBeLess                     = `"ttl" value must be less than "max_ttl" value`
	errorTextTPPandCloudMixedCredentials         = `TPP credentials and Cloud API key can't be specified in one role`
	errorTextStoreByAndStoreByCNOrSerialConflict = `Can't specify both story_by and store_by_cn or store_by_serial options '`
	errorTextNoStoreAndStoreByCNOrSerialConflict = `Can't specify both no_store and store_by_cn or store_by_serial options '`
	errorTextNoStoreAndStoreByConflict           = `Can't specify both no_store and store_by options '`
	errTextStoreByWrongOption                    = "Option store_by can be %s or %s, not %s"
)

func (b *backend) getRole(ctx context.Context, s logical.Storage, n string) (*roleEntry, error) {
	entry, err := s.Get(ctx, "role/"+n)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, nil
	}

	var result roleEntry
	if err := entry.DecodeJSON(&result); err != nil {
		return nil, err
	}

	return &result, nil
}

func (b *backend) pathRoleDelete(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	err := req.Storage.Delete(ctx, "role/"+data.Get("name").(string))
	if err != nil {
		return nil, err
	}

	return nil, nil
}

func (b *backend) pathRoleRead(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	roleName := data.Get("name").(string)
	if roleName == "" {
		return logical.ErrorResponse("missing role name"), nil
	}

	role, err := b.getRole(ctx, req.Storage, roleName)
	if err != nil {
		return nil, err
	}
	if role == nil {
		return nil, nil
	}

	resp := &logical.Response{
		Data: role.ToResponseData(),
	}
	return resp, nil
}

func (b *backend) pathRoleList(ctx context.Context, req *logical.Request, d *framework.FieldData) (*logical.Response, error) {
	entries, err := req.Storage.List(ctx, "role/")
	if err != nil {
		return nil, err
	}

	return logical.ListResponse(entries), nil
}

func (b *backend) pathRoleCreate(ctx context.Context, req *logical.Request, data *framework.FieldData) (*logical.Response, error) {
	var err error
	name := data.Get("name").(string)

	entry := &roleEntry{
		TPPURL:           data.Get("tpp_url").(string),
		CloudURL:         data.Get("cloud_url").(string),
		Zone:             data.Get("zone").(string),
		TPPPassword:      data.Get("tpp_password").(string),
		Apikey:           data.Get("apikey").(string),
		TPPUser:          data.Get("tpp_user").(string),
		TrustBundleFile:  data.Get("trust_bundle_file").(string),
		Fakemode:         data.Get("fakemode").(bool),
		ChainOption:      data.Get("chain_option").(string),
		StoreByCN:        data.Get("store_by_cn").(bool),
		StoreBySerial:    data.Get("store_by_serial").(bool),
		StoreBy:          data.Get("store_by").(string),
		NoStore:          data.Get("no_store").(bool),
		ServiceGenerated: data.Get("service_generated_cert").(bool),
		StorePrivateKey:  data.Get("store_pkey").(bool),
		KeyType:          data.Get("key_type").(string),
		KeyBits:          data.Get("key_bits").(int),
		KeyCurve:         data.Get("key_curve").(string),
		MaxTTL:           time.Duration(data.Get("max_ttl").(int)) * time.Second,
		TTL:              time.Duration(data.Get("ttl").(int)) * time.Second,
		GenerateLease:    data.Get("generate_lease").(bool),
		ServerTimeout:    time.Duration(data.Get("server_timeout").(int)) * time.Second,
	}

	err = validateEntry(entry)
	if err != nil {
		return logical.ErrorResponse(err.Error()), nil
	}

	// Store it
	jsonEntry, err := logical.StorageEntryJSON("role/"+name, entry)
	if err != nil {
		return nil, err
	}
	if err := req.Storage.Put(ctx, jsonEntry); err != nil {
		return nil, err
	}

	return nil, nil
}

func validateEntry(entry *roleEntry) (err error) {
	if !entry.Fakemode && entry.Apikey == "" && (entry.TPPURL == "" || entry.TPPUser == "" || entry.TPPPassword == "") {
		return fmt.Errorf(errorTextInvalidMode)
	}

	if entry.MaxTTL > 0 && entry.TTL > entry.MaxTTL {
		return fmt.Errorf(
			errorTextValueMustBeLess,
		)
	}

	if entry.TPPURL != "" && entry.Apikey != "" {
		return fmt.Errorf(errorTextTPPandCloudMixedCredentials)
	}

	if entry.TPPUser != "" && entry.Apikey != "" {
		return fmt.Errorf(errorTextTPPandCloudMixedCredentials)
	}

	if (entry.StoreByCN || entry.StoreBySerial) && entry.StoreBy != "" {
		return fmt.Errorf(errorTextStoreByAndStoreByCNOrSerialConflict)
	}

	if (entry.StoreByCN || entry.StoreBySerial) && entry.NoStore {
		return fmt.Errorf(errorTextNoStoreAndStoreByCNOrSerialConflict)
	}

	if entry.StoreBy != "" && entry.NoStore {
		return fmt.Errorf(errorTextNoStoreAndStoreByConflict)
	}

	if entry.StoreBy != "" {
		if (entry.StoreBy != storeBySerialString) && (entry.StoreBy != storeByCNString) {
			return fmt.Errorf(
				fmt.Sprintf(errTextStoreByWrongOption, storeBySerialString, storeByCNString, entry.StoreBy),
			)
		}
	}

	//StoreBySerial and StoreByCN options are deprecated
	//if one of them is set we will set store_by option
	//if both are set then we set store_by to serial
	if entry.StoreBySerial {
		entry.StoreBy = storeBySerialString
	} else if entry.StoreByCN {
		entry.StoreBy = storeByCNString
	}

	return nil
}

type roleEntry struct {

	//Venafi values
	TPPURL           string        `json:"tpp_url"`
	CloudURL         string        `json:"cloud_url"`
	Zone             string        `json:"zone"`
	TPPPassword      string        `json:"tpp_password"`
	Apikey           string        `json:"apikey"`
	TPPUser          string        `json:"tpp_user"`
	TrustBundleFile  string        `json:"trust_bundle_file"`
	Fakemode         bool          `json:"fakemode"`
	ChainOption      string        `json:"chain_option"`
	StoreByCN        bool          `json:"store_by_cn"`
	StoreBySerial    bool          `json:"store_by_serial"`
	StoreBy          string        `json:"store_by"`
	NoStore          bool          `json:"no_store"`
	ServiceGenerated bool          `json:"service_generated_cert"`
	StorePrivateKey  bool          `json:"store_pkey"`
	KeyType          string        `json:"key_type"`
	KeyBits          int           `json:"key_bits"`
	KeyCurve         string        `json:"key_curve"`
	LeaseMax         string        `json:"lease_max"`
	Lease            string        `json:"lease"`
	TTL              time.Duration `json:"ttl_duration"`
	MaxTTL           time.Duration `json:"max_ttl_duration"`
	GenerateLease    bool          `json:"generate_lease,omitempty"`
	DeprecatedMaxTTL string        `json:"max_ttl"`
	DeprecatedTTL    string        `json:"ttl"`
	ServerTimeout    time.Duration `json:"server_timeout"`
}

func (r *roleEntry) ToResponseData() map[string]interface{} {
	responseData := map[string]interface{}{
		//Venafi
		"tpp_url":   r.TPPURL,
		"cloud_url": r.CloudURL,
		"zone":      r.Zone,
		//We shouldn't show credentials
		//"tpp_password":      r.TPPPassword,
		//"apikey":            r.Apikey,
		"tpp_user":               r.TPPUser,
		"trust_bundle_file":      r.TrustBundleFile,
		"fakemode":               r.Fakemode,
		"store_by":               r.StoreBy,
		"no_store":               r.NoStore,
		"store_by_cn":            r.StoreByCN,
		"store_by_serial":        r.StoreBySerial,
		"service_generated_cert": r.ServiceGenerated,
		"store_pkey":             r.StorePrivateKey,
		"ttl":                    int64(r.TTL.Seconds()),
		"max_ttl":                int64(r.MaxTTL.Seconds()),
		"generate_lease":         r.GenerateLease,
		"chain_option":           r.ChainOption,
	}
	return responseData
}

const (
	pathListRolesHelpSyn  = `List the existing roles in this backend`
	pathListRolesHelpDesc = `Roles will be listed by the role name.`
	pathRoleHelpSyn       = `Manage the roles that can be created with this backend.`
	pathRoleHelpDesc      = `This path lets you manage the roles that can be created with this backend.`
)
