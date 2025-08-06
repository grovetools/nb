local M = {}

-- Get nb command from config
local function get_nb_command()
  local nb = require('nb')
  return nb.config.nb_command or "nb"
end

-- Get the current workspace context
function M.get_context()
  local nb_cmd = get_nb_command()
  local handle = io.popen(nb_cmd .. " context --json 2>/dev/null")
  if handle then
    local result = handle:read("*a")
    handle:close()
    
    -- Parse JSON response
    local ok, context = pcall(vim.json.decode, result)
    if ok then
      return context
    end
  end
  return nil
end

-- Run nb command and return output
function M.run_nb_command(cmd)
  local nb_cmd = get_nb_command()
  local full_cmd = nb_cmd .. " " .. cmd
  local handle = io.popen(full_cmd .. " 2>&1")
  if handle then
    local result = handle:read("*a")
    handle:close()
    return result
  end
  return nil
end

-- Run nb command and return output with separate error handling
function M.run_nb_command_with_error(cmd)
  local nb_cmd = get_nb_command()
  local full_cmd = nb_cmd .. " " .. cmd
  
  -- Create temporary file for stderr
  local stderr_file = vim.fn.tempname()
  local handle = io.popen(full_cmd .. " 2>" .. stderr_file)
  if handle then
    local stdout = handle:read("*a")
    local exit_success = handle:close()
    
    -- Read stderr
    local stderr_handle = io.open(stderr_file, "r")
    local stderr = ""
    if stderr_handle then
      stderr = stderr_handle:read("*a")
      stderr_handle:close()
    end
    
    -- Clean up temp file
    vim.fn.delete(stderr_file)
    
    return {
      stdout = stdout,
      stderr = stderr,
      success = exit_success ~= nil
    }
  end
  return { stdout = "", stderr = "Failed to execute command", success = false }
end

-- Get path for note type
function M.get_note_path(note_type)
  local context = M.get_context()
  if context and context.paths then
    return context.paths[note_type]
  end
  return nil
end

-- Copy text to clipboard
function M.copy_to_clipboard(text)
  vim.fn.setreg('+', text)
  vim.fn.setreg('"', text)
  vim.notify("Copied: " .. text)
end

-- Check if current repository is initialized
function M.is_repository_initialized()
  local context = M.get_context()
  if not context or not context.workspace then
    return false, "Not in a valid nb workspace"
  end
  
  -- Try to list notes to see if the repository is initialized
  local result = M.run_nb_command_with_error("list --json")
  if result.success and result.stdout ~= "" then
    local ok, notes = pcall(vim.json.decode, result.stdout)
    if ok then
      return true, nil
    end
  end
  
  -- Check if the error indicates uninitialized repository
  if result.stderr:match("no such file or directory") or 
     result.stderr:match("repository not initialized") or
     result.stdout == "" then
    return false, "Repository not initialized"
  end
  
  return false, "Unknown error: " .. (result.stderr or "")
end

-- Initialize repository
function M.init_repository()
  local result = M.run_nb_command_with_error("init")
  return result.success, result.stderr
end

return M