# systemdユーザーサービス管理CLI 要求仕様

## 1. 概要と目的

Pipeferryの`unix-listen`プロセスを、WSL上のsystemdユーザーサービスとして登録、起動、停止、削除できるCLIを追加する。

シェルの初期化ファイルからPipeferryプロセスを直接起動する方式を廃止し、次の責務をsystemdへ移す。

* WSL起動時の自動起動
* プロセスの監視
* 異常終了時の再起動
* ログの収集
* 停止と再起動
* 多重起動の防止

Pipeferryは汎用ツールであるため、SSH Agent専用の実装にはしない。

## 2. スコープ

今回実装するコマンドは次の3つとする。

```text
pipeferry service install
pipeferry service status
pipeferry service uninstall
```

サービス再起動やログ表示などの管理機能は、systemctlとjournalctlを直接利用するものとし、初期実装には含めない。

## 3. service install

### 3.1 コマンド形式

```text
pipeferry service install [options] -- executable [arguments...]
```

使用例:

```bash
pipeferry service install \
  --name ssh-agent \
  --socket-name ssh-agent.sock \
  -- \
  pipeferry.exe npipe-connect \
    --pipe openssh-ssh-agent \
    --connect-timeout 5s
```

### 3.2 オプション

```text
--name NAME
```

必須。

systemdサービス名を識別する論理名。

許可する文字は次のみとする。

```text
a-z
A-Z
0-9
-
_
```

生成されるユニット名:

```text
pipeferry-NAME.service
```

例:

```text
pipeferry-ssh-agent.service
```

```text
--socket-name NAME
```

必須。

`%t/pipeferry`以下に作成するUnix Domain Socketのファイル名。

ディレクトリ区切り文字、絶対パス、`..`は許可しない。

生成されるソケットパス:

```text
%t/pipeferry/SOCKET_NAME
```

例:

```text
/run/user/1000/pipeferry/ssh-agent.sock
```

```text
--shutdown-timeout DURATION
```

任意。

既定値:

```text
5s
```

`pipeferry unix-listen`へ渡す停止待機時間。

```text
--max-connections NUMBER
```

任意。

既定値:

```text
32
```

`pipeferry unix-listen`へ渡す最大同時接続数。

```text
--force
```

任意。

同名ユニットが存在する場合に上書きを許可する。

未指定で同名ユニットが存在する場合は、既存内容が同一であれば成功し、異なる場合はエラーとする。

### 3.3 子コマンド

`--`以降を、Unixソケット接続ごとに起動する子プロセスのargvとして保存する。

シェル文字列として解釈してはならない。

```bash
-- pipeferry.exe npipe-connect --pipe openssh-ssh-agent
```

次のようなシェル展開は行わない。

* パイプ
* リダイレクト
* コマンド置換
* 環境変数展開
* ワイルドカード展開

子実行ファイルが相対名の場合は、登録時にPATHから探索し、可能であれば絶対パスへ解決する。

## 4. service installの処理

コマンドは次の順序で処理する。

1. Linux上で実行されていることを確認
2. PID 1がsystemdであることを確認
3. `systemctl --user`が利用可能であることを確認
4. Pipeferry自身の絶対パスを取得
5. 子コマンドを検証
6. systemdユーザーユニットを生成
7. ユニットファイルを原子的に配置
8. `systemctl --user daemon-reload`を実行
9. `systemctl --user enable`を実行
10. `systemctl --user restart`を実行
11. サービスがactiveになったことを確認
12. Unixソケットが作成されたことを確認

途中で失敗した場合は、エラー内容と復旧方法を標準エラーへ出力する。

## 5. 生成するユニットファイル

配置先:

```text
${XDG_CONFIG_HOME:-$HOME/.config}/systemd/user/pipeferry-NAME.service
```

生成例:

```ini
[Unit]
Description=Pipeferry service ssh-agent
Documentation=https://github.com/masahide/pipeferry

[Service]
Type=simple
ExecStart=/home/example/.local/bin/pipeferry unix-listen --socket %t/pipeferry/ssh-agent.sock --shutdown-timeout 5s --max-connections 32 -- /mnt/c/Users/example/AppData/Local/Programs/pipeferry/pipeferry.exe npipe-connect --pipe openssh-ssh-agent --connect-timeout 5s
Restart=on-failure
RestartSec=2s

[Install]
WantedBy=default.target
```

### 必須設定

```ini
Type=simple
Restart=on-failure
RestartSec=2s
WantedBy=default.target
```

`ExecStart`にはPipeferry自身と子コマンドの絶対パスを使用する。

引数はsystemdの構文に従い、安全にエスケープする。

ユニットファイルへ次を含む値を直接埋め込んではならない。

* 改行
* NUL
* 制御文字
* 不正なsystemdエスケープ
* 未検証のユーザー入力

## 6. 冪等性

同じ設定で`service install`を複数回実行しても成功すること。

```bash
pipeferry service install ...
pipeferry service install ...
```

2回目以降も次を実行する。

```text
daemon-reload
enable
restart
```

これにより、Pipeferry本体や子実行ファイルが更新された場合も新しいプロセスへ切り替わる。

既存ユニットの内容が異なる場合は、`--force`なしでは上書きしない。

## 7. service status

### 7.1 コマンド形式

```text
pipeferry service status --name NAME
```

### 7.2 出力内容

最低限、次を表示する。

```text
Service
UnitFile
Enabled
Active
Socket
SocketState
```

出力例:

```text
Service: pipeferry-ssh-agent.service
UnitFile: /home/example/.config/systemd/user/pipeferry-ssh-agent.service
Enabled: yes
Active: yes
Socket: /run/user/1000/pipeferry/ssh-agent.sock
SocketState: live
```

### 7.3 JSON出力

既存CLIとの整合のため、`--json`をサポートする。

```bash
pipeferry service status --name ssh-agent --json
```

出力例:

```json
{
  "name": "ssh-agent",
  "unit": "pipeferry-ssh-agent.service",
  "unitFile": "/home/example/.config/systemd/user/pipeferry-ssh-agent.service",
  "enabled": true,
  "active": true,
  "socket": "/run/user/1000/pipeferry/ssh-agent.sock",
  "socketState": "live"
}
```

## 8. service uninstall

### 8.1 コマンド形式

```text
pipeferry service uninstall --name NAME
```

### 8.2 処理

次の順序で処理する。

1. `systemctl --user disable --now`を実行
2. ユニットファイルを削除
3. `systemctl --user daemon-reload`を実行
4. `systemctl --user reset-failed`を実行
5. 残存するUnixソケットとロックファイルを確認
6. 安全に削除可能なPipeferry所有ファイルだけ削除

対象ユニットが存在しない場合も成功とする。

Pipeferry本体、Windows側バイナリ、他のPipeferryサービスは削除しない。

## 9. systemdが利用できない場合

次のいずれかに該当する場合、登録処理を行わずエラーにする。

* Linux以外で実行された
* PID 1がsystemdではない
* `systemctl`が存在しない
* `systemctl --user`へ接続できない
* ユーザーランタイムディレクトリが利用できない

エラー例:

```text
pipeferry: systemd user services are not available

Enable systemd in /etc/wsl.conf:

[boot]
systemd=true

Then run the following command from Windows:

wsl --shutdown
```

Pipeferryバイナリ自体のインストールを取り消してはならない。

## 10. ログ

CLI自身の診断は標準エラーへ出力する。

常駐サービスのログはsystemd journalへ出力する。

確認方法をインストール完了時に表示する。

```bash
journalctl --user --unit pipeferry-ssh-agent.service --follow
```

転送するペイロードはログへ出力してはならない。

## 11. SSH_AUTH_SOCK

サービス登録機能は、シェル設定ファイルを自動変更しない。

インストール完了時に、設定すべき値を表示する。

Bashおよびzsh:

```bash
export SSH_AUTH_SOCK="${XDG_RUNTIME_DIR:-/run/user/$(id -u)}/pipeferry/ssh-agent.sock"
```

fish:

```fish
set -gx SSH_AUTH_SOCK /run/user/(id -u)/pipeferry/ssh-agent.sock
```

サービス登録とシェル環境変数設定は別責務とする。

## 12. 終了コード

既存の終了コード体系を維持する。

```text
0 成功
1 内部エラーまたはsystemd操作エラー
2 CLI引数エラー
3 子実行ファイルが見つからない
4 Unixソケットまたはユニットファイル操作エラー
7 同名サービスが異なる設定で既に存在する
8 サービス起動または停止のタイムアウト
9 サービス状態の検証失敗
```

## 13. 受け入れ条件

### AC-01 サービス登録

Given systemdが有効なWSL環境である
When `pipeferry service install`を実行する
Then ユーザーサービスが登録、有効化、起動される。

### AC-02 WSL再起動

Given サービスが有効化されている
When `wsl --shutdown`後にUbuntuを再度起動する
Then ユーザー操作なしでPipeferryリスナーが起動する。

### AC-03 SSH Agent接続

Given OpenSSH Agent向けサービスが起動している
When `SSH_AUTH_SOCK`を設定して`ssh-add -l`を実行する
Then WindowsのOpenSSH Agentから鍵一覧を取得できる。

### AC-04 冪等性

Given 同じサービスが登録済みである
When 同一引数で`service install`を再実行する
Then エラーにならず、サービスが再起動される。

### AC-05 設定衝突

Given 同名サービスが異なる設定で登録済みである
When `--force`なしで登録する
Then 既存設定を変更せずエラーを返す。

### AC-06 アンインストール

Given サービスが登録されている
When `service uninstall`を実行する
Then サービスが停止、無効化、削除される。

### AC-07 systemd未対応環境

Given systemdユーザーサービスが利用できない
When `service install`を実行する
Then ユニットを作成せず、具体的な有効化手順を表示する。

## 14. 非スコープ

初期実装では次を行わない。

* システム全体のsystemdサービス登録
* root権限でのサービス登録
* `/etc/systemd/system`の変更
* `/etc/wsl.conf`の自動変更
* `SSH_AUTH_SOCK`のシェル設定自動編集
* systemd socket activation
* 複数ユーザー間でのサービス共有
* Windows起動時にWSL自体を起動する処理
* journalログのラッパーコマンド
* systemd非対応環境向けの独自デーモン管理
