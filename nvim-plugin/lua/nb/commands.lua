local M = {}
local utils = require('nb.utils')

function M.setup(config)
  -- Create new note
  vim.api.nvim_create_user_command('NbNew', function(opts)
    local args = opts.args
    local cmd = "new"
    
    if args and args ~= "" then
      cmd = cmd .. " " .. vim.fn.shellescape(args)
    end
    
    -- Add --no-edit flag to prevent opening in external editor
    cmd = cmd .. " --no-edit"
    
    local result = utils.run_nb_command(cmd)
    if result then
      -- Extract the created file path
      local path = result:match("Created: (.+)")
      if path then
        path = vim.trim(path)
        vim.cmd("edit " .. path)
      else
        vim.notify("Failed to create note: " .. result, vim.log.levels.ERROR)
      end
    end
  end, { nargs = '?', desc = 'Create a new note' })

  -- Create quick note
  vim.api.nvim_create_user_command('NbQuick', function(opts)
    local content = opts.args
    
    if content == "" then
      -- Get content from user input
      content = vim.fn.input("Quick note: ")
      if content == "" then
        vim.notify("Quick note cancelled", vim.log.levels.INFO)
        return
      end
    end
    
    local cmd = "quick " .. vim.fn.shellescape(content)
    local result = utils.run_nb_command(cmd)
    if result then
      -- Extract the created file path
      local path = result:match("Created quick note: (.+)")
      if path then
        path = vim.trim(path)
        vim.cmd("edit " .. path)
      else
        vim.notify("Failed to create quick note: " .. result, vim.log.levels.ERROR)
      end
    end
  end, { nargs = '?', desc = 'Create a quick note' })

  -- List notes
  vim.api.nvim_create_user_command('NbList', function(opts)
    local note_type = opts.args ~= "" and opts.args or "current"
    local result = utils.run_nb_command("list -t " .. note_type .. " --json")
    if result then
      local ok, notes = pcall(vim.json.decode, result)
      if ok and notes then
        if #notes == 0 then
          vim.notify("No " .. note_type .. " notes found")
          return
        end
        
        -- Create quickfix items from notes
        local qf_items = {}
        for _, note in ipairs(notes) do
          table.insert(qf_items, {
            filename = note.path,
            text = string.format("[%s] %s (%d words)", note.type, note.title, note.word_count)
          })
        end
        
        vim.fn.setqflist(qf_items)
        vim.cmd('copen')
      else
        vim.notify("Failed to parse notes list", vim.log.levels.ERROR)
      end
    end
  end, { nargs = '?', desc = 'List notes' })

  -- Search notes
  vim.api.nvim_create_user_command('NbSearch', function(opts)
    if opts.args == "" then
      vim.notify("Search query required", vim.log.levels.ERROR)
      return
    end
    
    local result = utils.run_nb_command("search " .. vim.fn.shellescape(opts.args))
    if result then
      -- Parse search results and open in quickfix
      local lines = vim.split(result, '\n')
      local qf_items = {}
      
      for _, line in ipairs(lines) do
        local path = line:match("^(.+%.md)")
        if path then
          table.insert(qf_items, {
            filename = path,
            text = line
          })
        end
      end
      
      if #qf_items > 0 then
        vim.fn.setqflist(qf_items)
        vim.cmd('copen')
      else
        vim.notify("No results found")
      end
    end
  end, { nargs = 1, desc = 'Search notes' })

  -- Archive notes
  vim.api.nvim_create_user_command('NbArchive', function(opts)
    local args = opts.args
    
    -- If no arguments provided, archive the current file
    if args == "" then
      local current_file = vim.fn.expand('%:p')
      if current_file == "" then
        vim.notify("No file open to archive", vim.log.levels.ERROR)
        return
      end
      
      -- Check if it's a note file
      if not current_file:match("%.md$") then
        vim.notify("Current file is not a markdown note", vim.log.levels.ERROR)
        return
      end
      
      args = vim.fn.shellescape(current_file)
    end
    
    -- Always add --force flag to skip interactive prompt
    local result = utils.run_nb_command("archive --force " .. args)
    if result then
      vim.notify(result)
      
      -- If we archived the current file, close the buffer
      if args == vim.fn.shellescape(vim.fn.expand('%:p')) then
        vim.cmd('bdelete!')
      end
    end
  end, { nargs = '?', desc = 'Archive notes' })

  -- Move notes
  vim.api.nvim_create_user_command('NbMove', function(opts)
    local args = opts.args
    
    -- If no arguments provided, prompt for destination
    if args == "" then
      local current_file = vim.fn.expand('%:p')
      if current_file == "" then
        vim.notify("No file open to move", vim.log.levels.ERROR)
        return
      end
      
      -- Check if it's a note file
      if not current_file:match("%.md$") then
        vim.notify("Current file is not a markdown note", vim.log.levels.ERROR)
        return
      end
      
      -- Prompt for destination
      local dest = vim.fn.input("Move to (type/workspace): ")
      if dest == "" then
        vim.notify("Move cancelled", vim.log.levels.INFO)
        return
      end
      
      args = vim.fn.shellescape(current_file) .. " " .. dest
    end
    
    -- Run the move command
    local result = utils.run_nb_command("move " .. args)
    if result then
      vim.notify(result)
      
      -- If we moved the current file, reload it from the new location
      local new_path = result:match("To:%s+(.+)")
      if new_path then
        new_path = vim.trim(new_path)
        -- Close current buffer and open new location
        vim.cmd('bdelete!')
        vim.cmd('edit ' .. new_path)
      end
    end
  end, { nargs = '?', desc = 'Move notes to different locations' })

  -- Show workspace context
  vim.api.nvim_create_user_command('NbContext', function()
    local context = utils.get_context()
    if context then
      vim.notify(vim.inspect(context))
    else
      vim.notify("Failed to get context", vim.log.levels.ERROR)
    end
  end, { desc = 'Show workspace context' })
end

return M