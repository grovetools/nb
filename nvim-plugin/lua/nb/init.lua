local M = {}

-- Default configuration
M.config = {
	nb_command = "nb",
	mappings = {
		current = "<leader>ni",
		llm = "<leader>nc",
		learn = "<leader>nl",
		new = "<leader>nn",
		quick = "<leader>nq",
		path = "<leader>cp",
		search = "<leader>ns",
		search_global = "<leader>ng",
		search_all = "<leader>na",
		search_repo = "<leader>nb",
		archive = "<leader>nr",
		move = "<leader>nm",
	},
}

-- Setup function
function M.setup(opts)
	M.config = vim.tbl_deep_extend("force", M.config, opts or {})

	-- Load modules
	require("nb.commands").setup(M.config)
	require("nb.mappings").setup(M.config)
end

return M

