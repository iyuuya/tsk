# tsk

プロジェクト内のタスクを、タスクランナーの種類を問わず横断的に検出して一覧・実行できる CLI ツール。「このディレクトリはどのツールでタスクを実行するんだっけ」を覚えておく必要がなくなります。

どのファイルをどのタスクランナーとして扱うかはすべて設定で定義されており、組み込みのデフォルト設定は [`config/default.go`](config/default.go) にあります。

## インストール

Go(1.27 以降)があれば `go install` でインストールできます。

```
go install -ldflags="-s -w" -trimpath github.com/iyuuya/tsk@latest
```

ソースからインストールする場合:

```
git clone https://github.com/iyuuya/tsk
cd tsk
mise run install   # go install . を実行($GOBIN / ~/go/bin に入ります)
```

## 使い方

```
tsk
```

引数なしでは `tsk run` と同じ組み込みファジーピッカーを開きます。

```
tsk list [--refresh] [--scope global|repo|dir] [--json]
```

タスクを一覧します。`--refresh` でキャッシュを無視して再検出します。`--scope` で対象範囲を選べます:

- `repo` — カレントの git リポジトリ(`.git` のあるルート)配下を再帰的に集約(デフォルト)
- `dir` — コマンド実行時のカレントディレクトリ配下のみ(リポジトリ内であればキャッシュはリポジトリ単位で共有されます)
- `global` — tsk がキャッシュしている全プロジェクトのタスク(キャッシュのみを参照するため `--refresh` とは併用できません)

```
tsk run [--refresh] [--scope global|repo|dir] [<root>] [<dir>] [<adaptor> <task>]
```

指定したタスクを実行します。`adaptor` と `task` を省略すると、組み込みのファジーピッカーでタスクを検索・選択できます。入力するとリアルタイムに絞り込まれ、矢印キーまたは `Ctrl-P`/`Ctrl-N` で移動、`Enter` で実行、`Esc` または `Ctrl-C` で中止します。`--scope global` で引数を省略した場合は、キャッシュ済みの全プロジェクトから選択します。明示的に実行する `--scope global` では第 1 引数に対象プロジェクトの `<root>` を渡します。

## 設定

設定は `~/.config/tsk/config.toml`(`$XDG_CONFIG_HOME/tsk/config.toml`)から読み込まれます。ファイルが存在しない場合は組み込みのデフォルト設定が使われます。

`--scope` を指定しなかったときのデフォルトは、トップレベルの `default_scope` で変更できます(未設定なら `repo`):

```toml
default_scope = "repo"  # "global" | "repo" | "dir"
```

```
tsk config init [--force]
```

でデフォルト定義を設定ファイルとして書き出せます(既存ファイルは `--force` なしでは上書きしません)。これを編集して、アダプタの検出条件・実行コマンドの変更や、アダプタ自体の追加・削除ができます。

## Neovim プラグイン

`nvim/` に Neovim(0.10+)用プラグインが入っています。`:Tsk` でカレントプロジェクトのタスクを選んでターミナルで実行できます(`:Tsk!` はキャッシュを無視して再検出)。`:Tsk global` なら全プロジェクトのキャッシュ済みタスクを横断して選択でき、プロジェクトの外からでも実行できます(内部的には `tsk run --scope=global`)。選択には `fzf` があればフローティングターミナルの fzf を使い、なければ `vim.ui.select` にフォールバックします。実行結果はプロセス終了後も残る terminal-emulator バッファに表示されるので、スクロール・ヤンク・検索がそのままでき、末尾に終了ステータスが色付きで付きます(`q` でウィンドウを閉じる)。`tsk` コマンドが PATH にある必要があります。

[lazy.nvim](https://github.com/folke/lazy.nvim) はサブディレクトリのプラグインを直接扱えないため、`runtimepath` に `nvim/` を追加してから `setup()` を呼びます:

```lua
{
  "iyuuya/tsk",
  config = function(plugin)
    vim.opt.rtp:append(plugin.dir .. "/nvim")
    require("tsk").setup({
      -- cmd = "tsk",       -- tsk 実行ファイル(PATH にない場合は絶対パスを指定)
      -- open = "split",    -- タスク実行ターミナルの開き方: "split" | "vsplit" | "tab"
      -- picker = "auto",   -- タスク選択 UI: "auto"(fzf があれば fzf) | "fzf" | "select"
      -- fzf = { width = 0.8, height = 0.8 },  -- fzf フロートのサイズ(エディタ比)
    })
  end,
}
```

`nvim/` を起動時から `runtimepath` に入れる構成(packadd など)であれば `setup()` は省略できます。

## 開発

- ビルド(`bin/tsk` に出力): `mise run build`
- 実行: `mise run run`
- テスト: `go test ./...`
- Vet: `go vet ./...`
