-- tsk.nvim — pick and run tasks discovered by the tsk CLI from inside
-- Neovim. Requires Neovim 0.10+ (vim.system).

local M = {}

local defaults = {
  -- Executable used for every invocation; set to an absolute path if tsk
  -- isn't on Neovim's PATH.
  cmd = "tsk",
  -- How the terminal running a task opens: "split", "vsplit" or "tab".
  open = "split",
  -- Task picker: "auto" uses fzf in a floating terminal when an fzf
  -- executable exists and falls back to vim.ui.select otherwise;
  -- "fzf"/"select" force one of the two.
  picker = "auto",
  -- Floating fzf window size, as fractions of the editor size.
  fzf = { width = 0.8, height = 0.8 },
}

M.options = vim.deepcopy(defaults)

function M.setup(opts)
  M.options = vim.tbl_deep_extend("force", vim.deepcopy(defaults), opts or {})
  -- Re-defined here as well as in plugin/tsk.lua: installs that only add
  -- nvim/ to runtimepath after startup (e.g. lazy.nvim's config hook,
  -- which runs after plugin/ sourcing) never source plugin/tsk.lua.
  M.create_command()
end

-- term_start starts cmd as a terminal job in the current (empty) buffer,
-- papering over termopen's deprecation in 0.11.
local function term_start(cmd, opts)
  if vim.fn.has("nvim-0.11") == 1 then
    return vim.fn.jobstart(cmd, vim.tbl_extend("force", opts or {}, { term = true }))
  end
  return vim.fn.termopen(cmd, opts)
end

-- parse_list turns `tsk list --json` stdout into a list of task tables:
-- root (absolute project root), dir (relative to root), adaptor, name,
-- description. root is what makes running from outside the project
-- possible (tsk run --scope=global <root> <dir> <adaptor> <task>).
function M.parse_list(stdout)
  local ok, rows = pcall(vim.json.decode, stdout)
  if not ok or type(rows) ~= "table" then
    return {}
  end
  local tasks = {}
  for _, r in ipairs(rows) do
    tasks[#tasks + 1] = {
      root = r.root,
      dir = r.dir,
      adaptor = r.adaptor,
      name = r.task,
      description = r.description,
    }
  end
  return tasks
end

-- project_label shortens an absolute root to its last two path components
-- (ghq-style "user/repo"), mirroring tsk's own global-listing labels.
local function project_label(root)
  return vim.fn.fnamemodify(root, ":h:t") .. "/" .. vim.fn.fnamemodify(root, ":t")
end

-- list runs `tsk list --json` asynchronously in opts.cwd and calls
-- cb(err, tasks) on the main loop. opts.refresh maps to --refresh;
-- opts.global maps to --scope=global (every project's cached tasks —
-- refresh doesn't apply there and is ignored). The scope is always passed
-- explicitly: the task tables carry dirs relative to the repository root,
-- so a user's default_scope config must not leak into these invocations.
function M.list(opts, cb)
  opts = opts or {}
  local cmd = { M.options.cmd, "list", "--json" }
  if opts.global then
    cmd[#cmd + 1] = "--scope=global"
  else
    cmd[#cmd + 1] = "--scope=repo"
    if opts.refresh then
      cmd[#cmd + 1] = "--refresh"
    end
  end
  local ok, err = pcall(vim.system, cmd, { text = true, cwd = opts.cwd }, function(res)
    vim.schedule(function()
      if res.code ~= 0 then
        cb(vim.trim(res.stderr or ("exit status " .. res.code)))
        return
      end
      cb(nil, M.parse_list(res.stdout or ""))
    end)
  end)
  if not ok then
    -- vim.system throws synchronously when the executable doesn't exist.
    cb(err)
  end
end

-- run_count disambiguates the buffer name of each run.
local run_count = 0

-- run executes one task (a table as returned by list) with tsk resolving
-- the project from opts.cwd — or, with opts.global, from the task's own
-- root (tsk run --scope=global), so it works from any directory. Output
-- goes to a detached terminal-emulator
-- buffer (nvim_open_term) fed from a pty job — unlike a regular :terminal
-- buffer it survives the process exiting (no "press any key to close"), so
-- the result stays scrollable/yankable, with ANSI colors intact. Typed
-- input in terminal-mode is still forwarded to the task's pty, and "q"
-- closes the window. Returns the buffer.
function M.run(task, opts)
  opts = opts or {}
  local open = ({ split = "new", vsplit = "vnew", tab = "tabnew" })[M.options.open] or "new"
  vim.cmd(open)
  local win = vim.api.nvim_get_current_win()
  local buf = vim.api.nvim_get_current_buf()
  run_count = run_count + 1
  vim.api.nvim_buf_set_name(buf, ("tsk://%d/%s %s"):format(run_count, task.adaptor, task.name))

  local job
  local chan = vim.api.nvim_open_term(buf, {
    on_input = function(_, _, _, data)
      if job then
        pcall(vim.fn.chansend, job, data)
      end
    end,
  })
  local function emit(data)
    if data ~= "" then
      pcall(vim.api.nvim_chan_send, chan, data)
    end
  end

  -- Scope is pinned for the same reason as in M.list: task.dir is relative
  -- to the repository root.
  local cmd
  if opts.global then
    cmd = { M.options.cmd, "run", "--scope=global", task.root, task.dir, task.adaptor, task.name }
  else
    cmd = { M.options.cmd, "run", "--scope=repo", task.dir, task.adaptor, task.name }
  end

  -- Size the pty to the window's text area so the task's output wraps
  -- where the window does.
  local info = vim.fn.getwininfo(win)[1] or {}
  job = vim.fn.jobstart(cmd, {
    cwd = opts.cwd,
    pty = true,
    width = math.max((info.width or vim.o.columns) - (info.textoff or 0), 1),
    height = info.height,
    on_stdout = function(_, data)
      emit(table.concat(data, "\n"))
    end,
    on_exit = function(_, code)
      local color = code == 0 and "\27[32m" or "\27[31m"
      emit(("\r\n%s[tsk] %s %s: exit %d\27[0m\r\n"):format(color, task.adaptor, task.name, code))
    end,
  })
  if job <= 0 then
    vim.notify("tsk: failed to start: " .. M.options.cmd, vim.log.levels.ERROR)
    return buf
  end
  -- The pty job's channel, e.g. for sending the task input
  -- programmatically (chansend); terminal-mode typing reaches it via
  -- on_input above.
  vim.b[buf].tsk_job = job

  vim.keymap.set("n", "q", "<Cmd>close<CR>", { buffer = buf, nowait = true })
  vim.api.nvim_create_autocmd("BufWipeout", {
    buffer = buf,
    once = true,
    callback = function()
      vim.fn.jobstop(job)
    end,
  })
  -- With the cursor on the last line, the terminal buffer auto-follows new
  -- output.
  vim.api.nvim_win_set_cursor(win, { vim.api.nvim_buf_line_count(buf), 0 })
  return buf
end

local function format_item(t)
  local label = t.adaptor .. "  " .. t.name
  if t.project then
    label = t.project .. "  " .. label
  end
  if t.dir ~= "" and t.dir ~= "." then
    label = label .. "  (" .. t.dir .. ")"
  end
  if t.description ~= "" then
    label = label .. "  — " .. t.description
  end
  return label
end

-- pick_fzf runs fzf over the tasks in a centered floating terminal and
-- calls on_choice(task or nil). Each fzf input line carries the task's
-- index in a hidden first field (--with-nth=2..) so the selection maps
-- back without re-parsing the label.
local function pick_fzf(tasks, on_choice)
  local lines = {}
  for i, t in ipairs(tasks) do
    lines[i] = i .. "\t" .. format_item(t)
  end
  local infile = vim.fn.tempname()
  local outfile = vim.fn.tempname()
  vim.fn.writefile(lines, infile)

  local width = math.floor(vim.o.columns * M.options.fzf.width)
  local height = math.floor(vim.o.lines * M.options.fzf.height)
  local buf = vim.api.nvim_create_buf(false, true)
  local win = vim.api.nvim_open_win(buf, true, {
    relative = "editor",
    width = width,
    height = height,
    row = math.floor((vim.o.lines - height) / 2),
    col = math.floor((vim.o.columns - width) / 2),
    border = "rounded",
    title = " tsk ",
  })

  local shellcmd = ("fzf --delimiter='\\t' --with-nth=2.. < %s > %s"):format(
    vim.fn.shellescape(infile),
    vim.fn.shellescape(outfile)
  )
  term_start({ "sh", "-c", shellcmd }, {
    on_exit = function(_, code)
      vim.schedule(function()
        if vim.api.nvim_win_is_valid(win) then
          vim.api.nvim_win_close(win, true)
        end
        if vim.api.nvim_buf_is_valid(buf) then
          vim.api.nvim_buf_delete(buf, { force = true })
        end
        local choice
        if code == 0 then
          local out = vim.fn.readfile(outfile)
          local idx = out[1] and tonumber(out[1]:match("^(%d+)\t"))
          choice = idx and tasks[idx] or nil
        end
        vim.fn.delete(infile)
        vim.fn.delete(outfile)
        on_choice(choice)
      end)
    end,
  })
  vim.cmd.startinsert()
end

local function pick_select(tasks, on_choice)
  vim.ui.select(tasks, { prompt = "tsk run", format_item = format_item }, on_choice)
end

-- pick lists tasks, lets the user choose one (fzf or vim.ui.select per
-- options.picker), and runs it. opts.refresh bypasses tsk's cache;
-- opts.cwd overrides the directory the project is resolved from (default:
-- Neovim's current working directory); opts.global picks from every
-- project tsk has cached state for (entries gain a "user/repo" prefix) and
-- runs the choice via tsk run --scope=global.
function M.pick(opts)
  opts = opts or {}
  local cwd = opts.cwd or vim.fn.getcwd()
  M.list({ refresh = opts.refresh, global = opts.global, cwd = cwd }, function(err, tasks)
    if err then
      vim.notify("tsk: " .. err, vim.log.levels.ERROR)
      return
    end
    if #tasks == 0 then
      vim.notify("tsk: no tasks found", vim.log.levels.WARN)
      return
    end
    if opts.global then
      for _, t in ipairs(tasks) do
        t.project = project_label(t.root)
      end
    end
    local use_fzf = M.options.picker == "fzf"
      or (M.options.picker == "auto" and vim.fn.executable("fzf") == 1)
    local picker = use_fzf and pick_fzf or pick_select
    picker(tasks, function(choice)
      if choice then
        M.run(choice, { cwd = cwd, global = opts.global })
      end
    end)
  end)
end

-- create_command defines :Tsk (and redefines it harmlessly if it already
-- exists). Split out so both plugin/tsk.lua and setup() can call it — see
-- the comment in setup.
function M.create_command()
  vim.api.nvim_create_user_command("Tsk", function(args)
    M.pick({ refresh = args.bang, global = args.fargs[1] == "global" })
  end, {
    bang = true,
    nargs = "?",
    complete = function()
      return { "global" }
    end,
    desc = "Pick and run a task via tsk (:Tsk! refreshes the cache; :Tsk global spans all projects)",
  })
end

return M
