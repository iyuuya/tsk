-- Sourced when nvim/ is on runtimepath at startup; lazy.nvim-style setups
-- that append nvim/ later get the command from require("tsk").setup()
-- instead.
if vim.g.loaded_tsk then
  return
end
vim.g.loaded_tsk = true

require("tsk").create_command()
