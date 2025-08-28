local M = {}
local utils = require('nb.utils')

function M.setup(config)
  -- Helper function to create a new note with prompt
  local function create_note(note_type, is_global)
    local title = ""
    
    -- Daily notes don't need a title
    if note_type ~= "daily" then
      title = vim.fn.input("Note title: ")
      if title == "" then
        return -- User cancelled
      end
    end
    
    local cmd = "new"
    if title ~= "" then
      cmd = cmd .. " " .. vim.fn.shellescape(title)
    end
    if note_type then
      cmd = cmd .. " -t " .. note_type
    end
    if is_global then
      cmd = cmd .. " -g"
    end
    cmd = cmd .. " --no-edit"
    
    local result = utils.run_nb_command(cmd)
    if result then
      local path = result:match("Created: (.+)")
      if path then
        path = vim.trim(path)
        vim.cmd("edit " .. path)
      end
    end
  end

  -- Helper function to show global note type picker
  local function show_global_note_type_picker()
    -- Try to use snacks picker directly if available
    local has_snacks, snacks = pcall(require, 'snacks')
    if has_snacks and snacks.picker then
      -- Define global note types with descriptions
      local note_types = {
        { text = "📝 Current - General notes", type = "current" },
        { text = "⚡ Quick - Quick captures and thoughts", type = "quick" },
        { text = "🤖 LLM - Chat/AI session notes", type = "llm" },
        { text = "📚 Learn - Learning and study notes", type = "learn" },
        { text = "📅 Daily - Daily journal entries", type = "daily" },
        { text = "🐛 Issues - Bug reports and problem tracking", type = "issues" },
        { text = "🏗️  Architecture - Design and architecture notes", type = "architecture" },
        { text = "✅ Todos - Task lists and project planning", type = "todos" },
        { text = "✍️  Blog - Draft a new blog post", type = "blog" },
      }

      snacks.picker({
        title = "Select Global Note Type",
        items = note_types,
        format = "text",
        layout = {
          preset = "dropdown",
          preview = false,
          layout = {
            width = 80,
            height = 20,
          },
        },
        confirm = function(picker, item)
          picker:close()
          if item then
            create_note(item.type, true)
          end
        end,
      })
    else
      -- Fallback to vim.ui.select
      local note_types = {
        { type = "current", desc = "Current - General notes", icon = "📝" },
        { type = "quick", desc = "Quick - Quick captures and thoughts", icon = "⚡" },
        { type = "llm", desc = "LLM - Chat/AI session notes", icon = "🤖" },
        { type = "learn", desc = "Learn - Learning and study notes", icon = "📚" },
        { type = "daily", desc = "Daily - Daily journal entries", icon = "📅" },
        { type = "issues", desc = "Issues - Bug reports and problem tracking", icon = "🐛" },
        { type = "architecture", desc = "Architecture - Design and architecture notes", icon = "🏗️" },
        { type = "todos", desc = "Todos - Task lists and project planning", icon = "✅" },
        { type = "blog", desc = "Blog - Draft a new blog post", icon = "✍️" },
      }

      local items = {}
      for _, nt in ipairs(note_types) do
        table.insert(items, nt.icon .. " " .. nt.desc)
      end

      vim.ui.select(items, {
        prompt = "Select global note type:",
      }, function(choice, idx)
        if choice and idx then
          local selected = note_types[idx]
          create_note(selected.type, true)
        end
      end)
    end
  end

  -- Helper function to show note type picker
  local function show_note_type_picker()
    -- Try to use snacks picker directly if available
    local has_snacks, snacks = pcall(require, 'snacks')
    if has_snacks and snacks.picker then
      -- Define note types with descriptions
      local note_types = {
        { text = "📝 Current - General notes for current work", type = "current" },
        { text = "⚡ Quick - Quick captures and thoughts", type = "quick" },
        { text = "🤖 LLM - Chat/AI session notes", type = "llm" },
        { text = "📚 Learn - Learning and study notes", type = "learn" },
        { text = "📅 Daily - Daily journal entries", type = "daily" },
        { text = "🐛 Issues - Bug reports and problem tracking", type = "issues" },
        { text = "🏗️  Architecture - Design and architecture notes", type = "architecture" },
        { text = "✅ Todos - Task lists and project planning", type = "todos" },
        { text = "🌍 Global - General notes (not repo-specific)", type = "global" },
      }

      snacks.picker({
        title = "Select Note Type",
        items = note_types,
        format = "text",
        layout = {
          preset = "dropdown",
          preview = false,
          layout = {
            width = 80,
            height = 20,
          },
        },
        confirm = function(picker, item)
          picker:close()
          if item then
            if item.type == "global" then
              show_global_note_type_picker()
            else
              create_note(item.type, false)
            end
          end
        end,
      })
    else
      -- Fallback to vim.ui.select
      local note_types = {
        { type = "current", desc = "Current - General notes for current work", icon = "📝" },
        { type = "quick", desc = "Quick - Quick captures and thoughts", icon = "⚡" },
        { type = "llm", desc = "LLM - Chat/AI session notes", icon = "🤖" },
        { type = "learn", desc = "Learn - Learning and study notes", icon = "📚" },
        { type = "daily", desc = "Daily - Daily journal entries", icon = "📅" },
        { type = "issues", desc = "Issues - Bug reports and problem tracking", icon = "🐛" },
        { type = "architecture", desc = "Architecture - Design and architecture notes", icon = "🏗️" },
        { type = "todos", desc = "Todos - Task lists and project planning", icon = "✅" },
        { type = "global", desc = "Global - General notes (not repo-specific)", icon = "🌍" },
      }

      local items = {}
      for _, nt in ipairs(note_types) do
        table.insert(items, nt.icon .. " " .. nt.desc)
      end

      vim.ui.select(items, {
        prompt = "Select note type:",
      }, function(choice, idx)
        if choice and idx then
          local selected = note_types[idx]
          if selected.type == "global" then
            show_global_note_type_picker()
          else
            create_note(selected.type, false)
          end
        end
      end)
    end
  end

  -- Helper function to open oil.nvim at note directory
  local function open_oil(note_type)
    local path = utils.get_note_path(note_type)
    if path then
      -- Check if oil.nvim is available
      local ok, oil = pcall(require, 'oil')
      if ok then
        oil.open(path, { sort = { { "mtime", "desc" } } })
      else
        -- Fallback to netrw if oil.nvim not available
        vim.cmd("Explore " .. path)
      end
    else
      vim.notify("Cannot determine " .. note_type .. " path", vim.log.levels.ERROR)
    end
  end

  -- Generic function to create note picker with common configuration
  local function create_note_picker(opts)
    local has_snacks, snacks = pcall(require, 'snacks')
    if not (has_snacks and snacks.picker) then
      -- Fallback to quickfix or notify user
      vim.notify("snacks.nvim is required for this feature.", vim.log.levels.WARN)
      vim.cmd("NbSearch .")
      return
    end

    snacks.picker({
      title = opts.title,
      items = opts.items,
      format = "text",
      layout = {
        layout = {
          box = "vertical",
          width = 0.8,
          height = 0.8,
          border = "rounded",
          title = "{title} {live} {flags}",
          {
            box = "vertical",
            { win = "input", height = 1, border = "bottom" },
            { win = "list", border = "none" },
          },
          { win = "preview", height = 0.5, border = "top" },
        },
      },
      preview = function(ctx)
        if ctx.item and ctx.item.file then
          -- Read file content and display in preview window
          local ok, lines = pcall(vim.fn.readfile, ctx.item.file)
          if ok then
            ctx.preview:set_lines(lines)
            ctx.preview:highlight({ ft = "markdown" })
          end
        end
      end,
      confirm = function(picker, item)
        picker:close()
        if item and item.file then
          vim.cmd("edit " .. item.file)
        end
      end,
      actions = {
        archive_selected = function(picker, item)
          local items = picker:selected({ fallback = true })
          if #items == 0 then
            return
          end
          
          for _, item in ipairs(items) do
            if item.file then
              -- Archive the note
              utils.run_nb_command("archive --force " .. vim.fn.shellescape(item.file))
              
              -- Close buffer if open
              local bufnr = vim.fn.bufnr(item.file)
              if bufnr ~= -1 then
                vim.api.nvim_buf_delete(bufnr, { force = true })
              end
            end
          end
          
          vim.notify("Archived " .. #items .. " note(s)")
        end,
        move_selected = function(picker, item)
          local items = picker:selected({ fallback = true })
          if #items == 0 then
            return
          end
          
          -- Prompt for destination
          local dest = vim.fn.input("Move to (type/workspace): ")
          if dest == "" then
            vim.notify("Move cancelled", vim.log.levels.INFO)
            return
          end
          
          for _, item in ipairs(items) do
            if item.file then
              -- Move the note
              local result = utils.run_nb_command("move " .. vim.fn.shellescape(item.file) .. " " .. dest)
              
              -- Close buffer if open
              local bufnr = vim.fn.bufnr(item.file)
              if bufnr ~= -1 then
                vim.api.nvim_buf_delete(bufnr, { force = true })
              end
            end
          end
          
          vim.notify("Moved " .. #items .. " note(s) to " .. dest)
        end,
      },
      win = {
        input = {
          keys = {
            ["<C-x>"] = { "archive_selected", mode = { "n", "i" }, desc = "Archive Selected Notes" },
            ["<C-m>"] = { "move_selected", mode = { "n", "i" }, desc = "Move Selected Notes" },
          },
        },
        list = {
          keys = {
            ["<C-x>"] = { "archive_selected", desc = "Archive Selected Notes" },
            ["<C-m>"] = { "move_selected", desc = "Move Selected Notes" },
          },
        },
      },
    })
  end

  -- Helper function to show notes finder
  local function show_notes_finder()
    -- Get current context to show in title
    local context = utils.get_context()
    local title = "Notes"
    if context then
      title = string.format("Notes - %s", context.workspace or "unknown")
      if context.branch and context.branch ~= "" then
        title = title .. " (" .. context.branch .. ")"
      end
    end

    -- Run nb list to get all notes
    local result = utils.run_nb_command("list --all --json")
    if not result or result == "" then
      vim.notify("No notes found", vim.log.levels.INFO)
      return
    end

    -- Parse JSON output
    local ok, notes = pcall(vim.json.decode, result)
    if not ok or not notes then
      vim.notify("Failed to parse notes list", vim.log.levels.ERROR)
      return
    end

    -- First, calculate max width for the note type column
    local max_type_width = 0
    for _, note in ipairs(notes) do
      local note_type = note.type or "unknown"
      local note_type_len = #note_type
      if note_type_len > max_type_width then
        max_type_width = note_type_len
      end
    end

    -- Add 3 for brackets and a space
    local format_str = string.format("%%s %%-%ds  %%s  %%s", max_type_width + 3)

    -- Convert notes to picker items
    local items = {}
    for _, note in ipairs(notes) do
      local path = note.path
      if path then
        -- Use data from the note object
        local note_type = note.type
        local title = note.title or "untitled"
        local filename = path:match("([^/]+)$") or path
        
        -- Get emoji for note type (use base type for nested paths)
        local base_type = note_type:match("^([^/]+)") or note_type
        local type_emojis = {
          current = "📝",
          quick = "⚡",
          llm = "🤖",
          learn = "📚",
          daily = "📅",
          archive = "📦",
          issues = "🐛",
          architecture = "🏗️",
          todos = "✅",
          blog = "✍️",
        }
        local emoji = type_emojis[base_type] or "📄"
        
        -- Use modification time from note object
        local mtime = 0
        if note.modified_at then
          -- Parse ISO timestamp
          local year, month, day, hour, min, sec = note.modified_at:match("(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)")
          if year then
            mtime = os.time({year=year, month=month, day=day, hour=hour, min=min, sec=sec})
          end
        end
        
        -- Format the display text with dynamic width columns
        local formatted_mtime = ""
        if mtime > 0 then
          formatted_mtime = os.date("%Y-%m-%d %H:%M", mtime)
        end
        
        local display_text = string.format(format_str, emoji, "[" .. note_type .. "]", formatted_mtime, title)
        
        table.insert(items, {
          text = display_text,
          file = path,
          note_type = note_type,
          filename = filename,
          date = note.created_at,
          title = title,
          mtime = mtime,
          word_count = note.word_count,
        })
      end
    end

    -- Sort items by modification time (newest first)
    table.sort(items, function(a, b)
      return (a.mtime or 0) > (b.mtime or 0)
    end)

    -- Call the generic picker
    create_note_picker({ title = title, items = items })
  end

  -- Helper function to show global notes finder
  local function show_global_notes_finder()
    -- Run nb list to get global notes
    local result = utils.run_nb_command("list -g --all --json")
    if not result or result == "" then
      vim.notify("No global notes found", vim.log.levels.INFO)
      return
    end

    -- Parse JSON output
    local ok, notes = pcall(vim.json.decode, result)
    if not ok or not notes then
      vim.notify("Failed to parse notes list", vim.log.levels.ERROR)
      return
    end

    -- First, calculate max width for the note type column
    local max_type_width = 0
    for _, note in ipairs(notes) do
      local note_type = note.type or "unknown"
      local note_type_len = #note_type
      if note_type_len > max_type_width then
        max_type_width = note_type_len
      end
    end

    -- Add 3 for brackets and a space
    local format_str = string.format("%%s %%-%ds  %%s  %%s", max_type_width + 3)

    -- Convert notes to picker items
    local items = {}
    for _, note in ipairs(notes) do
      local path = note.path
      if path then
        -- Use data from the note object
        local note_type = note.type
        local title = note.title or "untitled"
        local filename = path:match("([^/]+)$") or path
        
        -- Get emoji for note type (use base type for nested paths)
        local base_type = note_type:match("^([^/]+)") or note_type
        local type_emojis = {
          current = "📝",
          quick = "⚡",
          llm = "🤖",
          learn = "📚",
          daily = "📅",
          archive = "📦",
          issues = "🐛",
          architecture = "🏗️",
          todos = "✅",
          blog = "✍️",
        }
        local emoji = type_emojis[base_type] or "📄"
        
        -- Use modification time from note object
        local mtime = 0
        if note.modified_at then
          -- Parse ISO timestamp
          local year, month, day, hour, min, sec = note.modified_at:match("(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)")
          if year then
            mtime = os.time({year=year, month=month, day=day, hour=hour, min=min, sec=sec})
          end
        end
        
        -- Format the display text with dynamic width columns
        local formatted_mtime = ""
        if mtime > 0 then
          formatted_mtime = os.date("%Y-%m-%d %H:%M", mtime)
        end
        
        local display_text = string.format(format_str, emoji, "[" .. note_type .. "]", formatted_mtime, title)
        
        table.insert(items, {
          text = display_text,
          file = path,
          note_type = note_type,
          filename = filename,
          date = note.created_at,
          title = title,
          mtime = mtime,
          word_count = note.word_count,
        })
      end
    end

    -- Sort items by modification time (newest first)
    table.sort(items, function(a, b)
      return (a.mtime or 0) > (b.mtime or 0)
    end)

    -- Call the generic picker
    create_note_picker({ title = "Global Notes", items = items })
  end

  -- Helper function to show all notes finder (global + repo)
  local function show_all_notes_finder()
    -- Use nb list --workspaces --json to get notes from all workspaces
    local result = utils.run_nb_command("list --workspaces --json")
    if not result or result == "" then
      vim.notify("No notes found", vim.log.levels.INFO)
      return
    end

    -- Parse JSON output
    local ok, notes = pcall(vim.json.decode, result)
    if not ok or not notes then
      vim.notify("Failed to parse notes list", vim.log.levels.ERROR)
      return
    end

    -- First pass: calculate max widths for the note type and location columns
    local max_type_width = 0
    local max_location_width = 0
    for _, note in ipairs(notes) do
      local note_type = note.type or "unknown"
      local workspace = note.workspace or "global"
      local branch = note.branch or ""
      local location = workspace
      if branch ~= "" then
        location = location .. "/" .. branch
      end
      
      if #note_type > max_type_width then
        max_type_width = #note_type
      end
      if #location > max_location_width then
        max_location_width = #location
      end
    end

    -- Add padding for brackets and spacing
    local format_str = string.format("%%s %%-%ds %%-%ds %%s  %%s%%s", max_type_width + 3, max_location_width + 2)

    -- Convert notes to picker items
    local items = {}
    for _, note in ipairs(notes) do
      local path = note.path
      if path then
        -- Use data from the note object
        local note_type = note.type or "unknown"
        local title = note.title or "untitled"
        local workspace = note.workspace or "global"
        local branch = note.branch or ""
          local tags = note.tags or {}
          
          -- Get emoji for note type (use base type for nested paths)
          local base_type = note_type:match("^([^/]+)") or note_type
          local type_emojis = {
            current = "📝",
            quick = "⚡",
            llm = "🤖",
            learn = "📚",
            daily = "📅",
            archive = "📦",
            issues = "🐛",
            architecture = "🏗️",
            todos = "✅",
          }
          local emoji = type_emojis[base_type] or "📄"
          
          -- Use modification time from note object
          local mtime = 0
          if note.modified_at then
            -- Parse ISO timestamp
            local year, month, day, hour, min, sec = note.modified_at:match("(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)")
            if year then
              mtime = os.time({year=year, month=month, day=day, hour=hour, min=min, sec=sec})
            end
          end
          
          -- Build workspace/branch display
          local location = workspace
          if branch ~= "" then
            location = location .. "/" .. branch
          end
          
          -- Build tags display
          local tags_str = ""
          if #tags > 0 then
            tags_str = " [" .. table.concat(tags, ", ") .. "]"
          end
          
          -- Format modification time
          local formatted_mtime = ""
          if mtime > 0 then
            formatted_mtime = os.date("%Y-%m-%d", mtime)
          end
          
          -- Format with dynamic widths
          local display_text = string.format(format_str, 
            emoji, 
            "[" .. note_type .. "]", 
            location,
            formatted_mtime, 
            title,
            tags_str)
          
          table.insert(items, {
            text = display_text,
            file = path,
            note_type = note_type,
            filename = path:match("([^/]+)$") or path,
            date = note.created_at,
            title = title,
            is_global = workspace == "global",
            repo_name = workspace,
            branch_name = branch,
            mtime = mtime,
            tags = tags,
            id = note.id,
          })
        end
      end
      
      if #items == 0 then
        vim.notify("No notes found", vim.log.levels.INFO)
        return
      end

      -- Sort items by modification time (newest first)
      table.sort(items, function(a, b)
        return (a.mtime or 0) > (b.mtime or 0)
      end)

      -- Call the generic picker
      create_note_picker({ title = "All Notes (Global + All Repositories)", items = items })
  end

  -- Helper function to show repository-wide notes finder
  local function show_repo_notes_finder()
    -- Get current repository name for the title
    local context = utils.get_context()
    if not (context and context.workspace) then
      vim.notify("Not in a valid nb workspace.", vim.log.levels.WARN)
      return
    end
    
    local title = string.format("Repository Notes - %s", context.workspace)
    
    -- Check if repository is initialized
    local is_initialized, init_error = utils.is_repository_initialized()
    if not is_initialized then
      if init_error == "Repository not initialized" then
        -- Offer to initialize
        vim.ui.select(
          {"Yes", "No"},
          {
            prompt = string.format("Repository '%s' is not initialized for nb. Initialize now?", context.workspace),
          },
          function(choice)
            if choice == "Yes" then
              local success, err = utils.init_repository()
              if success then
                vim.notify("Repository initialized successfully. Creating your first note...", vim.log.levels.INFO)
                -- Create first note
                vim.schedule(function()
                  require('nb').new_note('current')
                end)
              else
                vim.notify("Failed to initialize repository: " .. (err or "unknown error"), vim.log.levels.ERROR)
              end
            end
          end
        )
        return
      else
        vim.notify("Repository check failed: " .. (init_error or "unknown error"), vim.log.levels.ERROR)
        return
      end
    end
    
    -- Run nb list with the new flag
    local result = utils.run_nb_command_with_error("list --all-branches --json")
    if not result.success or not result.stdout or result.stdout == "" then
      if result.stderr and result.stderr ~= "" then
        vim.notify("Error listing notes: " .. result.stderr, vim.log.levels.ERROR)
      else
        vim.notify("No notes found in this repository", vim.log.levels.INFO)
      end
      return
    end

    -- Parse JSON output
    local ok, notes = pcall(vim.json.decode, result.stdout)
    if not ok or not notes then
      vim.notify("Failed to parse repository notes list", vim.log.levels.ERROR)
      return
    end

    if #notes == 0 then
      vim.notify("No notes found in this repository", vim.log.levels.INFO)
      return
    end

    -- First pass: calculate max widths for note type and branch columns
    local max_type_width = 0
    local max_branch_width = 0
    for _, note in ipairs(notes) do
      local note_type = note.type or "unknown"
      local branch = note.branch or "main"
      
      if #note_type > max_type_width then
        max_type_width = #note_type
      end
      if #branch > max_branch_width then
        max_branch_width = #branch
      end
    end

    -- Add padding for brackets "()" and "[]"
    local type_col_width = max_type_width + 2 -- for "[]"
    local branch_col_width = max_branch_width + 2 -- for "()"
    local format_str = string.format("%%s %%-%ds %%-%ds %%s  %%s", type_col_width, branch_col_width)

    -- Convert notes to picker items
    local items = {}
    for _, note in ipairs(notes) do
      local path = note.path
      if path then
        -- This logic can be copied and adapted from show_all_notes_finder
        local note_type = note.type or "unknown"
        local title_text = note.title or "untitled"
        local branch = note.branch or "main"
        
        local base_type = note_type:match("^([^/]+)") or note_type
        local type_emojis = {
          current = "📝",
          quick = "⚡",
          llm = "🤖",
          learn = "📚",
          daily = "📅",
          archive = "📦",
          issues = "🐛",
          architecture = "🏗️",
          todos = "✅",
          blog = "✍️",
        }
        local emoji = type_emojis[base_type] or "📄"
        
        local mtime = 0
        if note.modified_at then
          local year, month, day, hour, min, sec = note.modified_at:match("(%d+)-(%d+)-(%d+)T(%d+):(%d+):(%d+)")
          if year then
              mtime = os.time({year=year, month=month, day=day, hour=hour, min=min, sec=sec})
          end
        end
        
        local formatted_mtime = ""
        if mtime > 0 then
          formatted_mtime = os.date("%Y-%m-%d %H:%M", mtime)
        end
        
        -- Format with dynamic widths
        local display_text = string.format(format_str, 
          emoji, 
          "[" .. note_type .. "]", 
          "(" .. branch .. ")", 
          formatted_mtime, 
          title_text)
        
        table.insert(items, {
          text = display_text,
          file = path,
          note_type = note_type,
          title = title_text,
          branch_name = branch,
          mtime = mtime,
        })
      end
    end
      
    -- Sort items by modification time (newest first)
    table.sort(items, function(a, b)
      return (a.mtime or 0) > (b.mtime or 0)
    end)

    -- Call the generic picker
    create_note_picker({ title = title, items = items })
  end

  -- Set up key mappings
  local mappings = config.mappings

  -- Open current notes
  if mappings.current then
    vim.keymap.set("n", mappings.current, function()
      open_oil("current")
    end, { desc = "Open current notes" })
  end

  -- Open LLM notes
  if mappings.llm then
    vim.keymap.set("n", mappings.llm, function()
      open_oil("llm")
    end, { desc = "Open LLM notes" })
  end

  -- Open learning notes
  if mappings.learn then
    vim.keymap.set("n", mappings.learn, function()
      open_oil("learn")
    end, { desc = "Open learning notes" })
  end

  -- Create new note
  if mappings.new then
    vim.keymap.set("n", mappings.new, function()
      show_note_type_picker()
    end, { desc = "Create new note" })
  end

  -- Create quick note
  if mappings.quick then
    vim.keymap.set("n", mappings.quick, function()
      local content = vim.fn.input("Quick note: ")
      if content ~= "" then
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
      end
    end, { desc = "Create quick note" })
  end

  -- Copy current file path to clipboard
  if mappings.path then
    vim.keymap.set("n", mappings.path, function()
      local path = vim.fn.expand('%:p')
      if path ~= "" then
        utils.copy_to_clipboard(path)
      else
        vim.notify("No file open", vim.log.levels.WARN)
      end
    end, { desc = "Copy file path" })
  end

  -- Search notes
  if mappings.search then
    vim.keymap.set("n", mappings.search, function()
      show_notes_finder()
    end, { desc = "Search notes" })
  end

  -- Search global notes
  if mappings.search_global then
    vim.keymap.set("n", mappings.search_global, function()
      show_global_notes_finder()
    end, { desc = "Search global notes" })
  end

  -- Search all notes (global + repo)
  if mappings.search_all then
    vim.keymap.set("n", mappings.search_all, function()
      show_all_notes_finder()
    end, { desc = "Search all notes" })
  end

  -- Search all notes in current repository
  if mappings.search_repo then
    vim.keymap.set("n", mappings.search_repo, function()
      show_repo_notes_finder()
    end, { desc = "Search all notes in repository" })
  end

  -- Archive notes
  if mappings.archive then
    vim.keymap.set("n", mappings.archive, function()
      -- Check if we're in a markdown file
      local current_file = vim.fn.expand('%:p')
      if current_file ~= "" and current_file:match("%.md$") then
        -- Ask for confirmation to archive current file
        local confirm = vim.fn.confirm("Archive current note?", "&Yes\n&No\n&Older notes...", 2)
        if confirm == 1 then
          vim.cmd("NbArchive")
        elseif confirm == 3 then
          local days = vim.fn.input("Archive notes older than (days) [30]: ")
          if days == "" then
            days = "30"
          end
          vim.cmd("NbArchive --older-than " .. days)
        end
      else
        -- Not in a note file, just do older notes
        local days = vim.fn.input("Archive notes older than (days) [30]: ")
        if days == "" then
          days = "30"
        end
        vim.cmd("NbArchive --older-than " .. days)
      end
    end, { desc = "Archive notes" })
  end

  -- Move notes
  if mappings.move then
    vim.keymap.set("n", mappings.move, function()
      vim.cmd("NbMove")
    end, { desc = "Move current note" })
  end

  -- Additional quick note creation mappings
  vim.keymap.set("n", "<leader>nnl", function()
    create_note("llm")
  end, { desc = "Create LLM note" })

  vim.keymap.set("n", "<leader>nnd", function()
    create_note("daily")
  end, { desc = "Create daily note" })

  vim.keymap.set("n", "<leader>nnr", function()
    create_note("learn")
  end, { desc = "Create learning note" })

  vim.keymap.set("n", "<leader>nnb", function()
    create_note("blog", true)
  end, { desc = "Create new Blog Post" })

  -- Global note creation
  vim.keymap.set("n", "<leader>nng", function()
    show_global_note_type_picker()
  end, { desc = "Create global note" })
end

return M