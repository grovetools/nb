" nb.nvim - Neovim plugin for nb note system
" This file ensures the plugin is loaded

if exists('g:loaded_nb')
  finish
endif
let g:loaded_nb = 1

" Ensure Lua module is available
lua << EOF
local ok, nb = pcall(require, 'nb')
if not ok then
  vim.notify("Failed to load nb.nvim: " .. tostring(nb), vim.log.levels.ERROR)
end
EOF