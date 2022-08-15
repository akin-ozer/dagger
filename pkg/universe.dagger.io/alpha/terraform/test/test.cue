package terraform

import (
	"dagger.io/dagger"
	"dagger.io/dagger/core"
	"universe.dagger.io/alpha/terraform"
)

// Set a terraform version for testing specific version of terraform
_#TerraformVersion: "1.2.6"

dagger.#Plan & {
	actions: test: {
		tfSource: core.#Source & {
			path: "./data"
		}

		tfImportSource: core.#Source & {
			path: "./import_data"
		}

		applyWorkflow: {
			init: terraform.#Init & {
				source: tfSource.output
			}

			validate: terraform.#Validate & {
				source: init.output
			}

			plan: terraform.#Plan & {
				source: validate.output
			}

			apply: terraform.#Apply & {
				source: plan.output
			}

			verify: #AssertFile & {
				input:    apply.output
				path:     "./out.txt"
				contents: "Hello, world!"
			}
		}

		destroyWorkflow: {
			destroy: terraform.#Destroy & {
				source: applyWorkflow.apply.output
			}

			// TODO assert out.txt doesn't exist
		}

		importWorkflow: {
			init: terraform.#Init & {
				source: tfImportSource.output
			}
			importResource: terraform.#Import & {
				source:  init.output
				address: "random_uuid.test"
				id:      "aabbccdd-eeff-0011-2233-445566778899"
			}
		}

		workspaceWorkflow: {
			init: terraform.#Init & {
				source: tfImportSource.output
			}

			workspaceNew: terraform.#Workspace & {
				source: init.output
				subCmd: "new"
				name:   "TEST_WORKSPACE"
			}

			workspaceList: terraform.#Workspace & {
				source: workspaceNew.output
				subCmd: "list"
			}

			workspaceShow: terraform.#Workspace & {
				source: workspaceNew.output
				subCmd: "show"
				name:   "TEST_WORKSPACE"
			}

			workspaceShowNoSubCmd: terraform.#Workspace & {
				source: workspaceNew.output
				subCmd: "show"
			}

			workspaceSelect: terraform.#Workspace & {
				source: workspaceNew.output
				subCmd: "select"
				name:   "default"
			}

			workspaceDelete: terraform.#Workspace & {
				source: workspaceSelect.output
				subCmd: "delete"
				name:   "TEST_WORKSPACE"
			}
		}

		versionWorkflow: {
			init: terraform.#Init & {
				source:  tfSource.output
				version: _#TerraformVersion
			}

			workspaceNew: terraform.#Workspace & {
				source:  init.output
				subCmd:  "new"
				name:    "TEST_WORKSPACE"
				version: _#TerraformVersion
			}

			plan: terraform.#Plan & {
				source:  init.output
				version: _#TerraformVersion
			}

			apply: terraform.#Apply & {
				always:  true
				source:  plan.output
				version: _#TerraformVersion
			}

			verify: #AssertFileRegex & {
				input: apply.output
				path:  "terraform.tfstate"
				regex: "1\\.2\\.6"
			}

			destroy: terraform.#Destroy & {
				source:  apply.output
				version: _#TerraformVersion
			}
		}

	}
}

// Make an assertion on the contents of a file
#AssertFile: {
	input:    dagger.#FS
	path:     string
	contents: string

	_read: core.#ReadFile & {
		"input": input
		"path":  path
	}

	actual: _read.contents

	// Assertion
	contents: actual
}

// Make an assertion on the contents of a file
#AssertFileRegex: {
	input: dagger.#FS
	path:  string
	regex: string

	_read: core.#ReadFile & {
		"input": input
		"path":  path
	}

	actual: _read.contents

	contents: =~regex
	// Assertion
	contents: actual
}
