# Pipeferry 初期実装 要求仕様

## 文書情報

| 項目 | 内容 |
|---|---|
| 文書名 | Pipeferry 初期実装 要求仕様 |
| 対象 | `pipeferry unix-listen` と `pipeferry.exe npipe-connect` |
| 作成日 | 2026-07-20 |
| 対象OS | Windows 11 と WSL2 |
| 実装言語 | Go |
| 状態 | 初期要求仕様 |

## 1. 背景

Pipeferryは、WSL2上のUnix Domain SocketとWindows上のNamed Pipeを、標準入出力を介して接続する汎用ストリームブリッジとして新規実装する。開発の起点は、OmniSSHAgentやWindows OpenSSH AgentをWSL2から利用するための既存実装である。

- WSL側のUnix Domain SocketをGoプロセスで待ち受ける
- WSLからPowerShellを起動する
- PowerShellからWindows Named Pipeへ接続する
- GoとPowerShellの標準入出力上で独自プロトコルを使い、複数接続を多重化する

また、過去には`Socat`からWindows用Goバイナリを接続ごとに起動し、Unix Domain SocketとNamed Pipeを中継する方式も存在していた。

現行方式には次の課題がある。

- PowerShellスクリプトの責務が大きい
- GoとPowerShellの両方に通信制御が分散している
- 独自の多重化プロトコルが必要になる
- 部分読み込み、終了処理、並行書き込みなどの実装が複雑になる
- PowerShellの実行環境や実行ポリシーの影響を受ける
- 障害発生時に原因の切り分けが難しい
- WSL側で`Socat`などの追加パッケージが必要になる方式もある

今回の新規実装では、Linux側とWindows側の双方をGoで一から作り直し、一つの`pipeferry`リポジトリと実行ファイルへ統合する。

## 2. 目的

本機能の目的は、WSL2内のUnix Domain SocketクライアントとWindows上のNamed Pipeサービスを、単純で堅牢な双方向ストリームとして接続することである。SSH Agent連携を主要ユースケースとするが、通信内容は限定しない。

具体的には次を実現する。

1. WSL2上に指定されたUnix Domain Socketを提供する
2. Unix Domain Socketへの接続ごとにWindows側子コマンドを起動する
3. WSL側ソケットとWindowsプロセスの標準入出力を双方向に中継する
4. Windowsプロセスの標準入出力とWindows Named Pipeを双方向に中継する
5. PowerShellと`Socat`への依存をなくす
6. SSH Agentプロトコルを中継プロセス内で解釈せず、そのまま転送する
7. 接続終了時に関連するソケット、パイプ、子プロセスを確実に終了する
8. インストーラーから安全に配置、更新できる構成にする

## 3. 対象範囲

### 3.1 対象に含めるもの

- Linux側コマンド `pipeferry unix-listen`
- Windows側コマンド `pipeferry.exe npipe-connect`
- Unix Domain Socketの作成と管理
- Windows子プロセスの起動と監視
- 標準入力と標準出力を使った双方向転送
- Windows Named Pipeへの接続
- 正常終了、異常終了、シグナル処理
- ログ出力
- 設定用のコマンドライン引数と環境変数
- 単体テスト、統合テスト、E2Eテスト
- 稼働確認用の診断コマンド

### 3.2 対象に含めないもの

- 接続先サービスのUI再実装
- タスクトレイ実装
- PowerShellワンライナーインストーラー本体
- 自動アップデート処理
- SSH秘密鍵の保存と管理
- 中継対象プロトコルの解析や書き換え
- 複数接続を一本の標準入出力へ集約する多重化
- Windowsサービス化
- WSLのsystemdユーザーサービス登録
- WSL1、MSYS2、Cygwin対応

これらは後続フェーズで扱う。

## 4. 全体構成

```text
LinuxまたはWSL2

Unix socket client
      │
      ▼
Unix Domain Socket
      │
      ▼
pipeferry unix-listen
      │
      │ 接続ごとに子プロセスを起動
      │ stdin と stdout
      ▼
pipeferry.exe npipe-connect

Windows

pipeferry.exe npipe-connect
      │
      │ Windows Named Pipe
      ▼
Windows named-pipe service

主な利用例

WSL2 OpenSSH client
      │ SSH_AUTH_SOCK
      ▼
Windows OpenSSH Agent、OmniSSHAgent、1Password SSH Agent
```

## 5. 基本設計方針

### 5.1 接続ごとにWindowsプロセスを起動する

初期実装では、Unix Domain Socketの接続ごとに`pipeferry.exe npipe-connect`を一つ起動する。

```text
Unix接続1 ─ pipeferry.exe 1 ─ Named Pipe接続1
Unix接続2 ─ pipeferry.exe 2 ─ Named Pipe接続2
Unix接続3 ─ pipeferry.exe 3 ─ Named Pipe接続3
```

この方式により、接続識別子、フレームヘッダー、チャネル管理などの独自多重化処理を不要にする。

### 5.2 通信内容を解釈しない

両プロセスはSSH Agentプロトコルのメッセージ境界を解釈しない。

すべての通信を透過的なバイトストリームとして扱う。

```text
Unix Socket → stdin → Named Pipe
Unix Socket ← stdout ← Named Pipe
```

### 5.3 標準出力をデータ専用にする

`pipeferry.exe npipe-connect`の標準出力には、Named Pipeから受信したバイナリデータ以外を一切出力してはならない。

ログ、警告、診断情報、エラーはすべて標準エラーへ出力する。

### 5.4 責務を最小化する

WSL側はUnix Domain Socketと子プロセスを管理する。

Windows側は標準入出力とNamed Pipeだけを中継する。

特定サービスの設定ファイル、Windows Credential Manager、鍵情報をPipeferryから直接参照しない。

## 6. コマンド概要

## 6.1 `pipeferry unix-listen`

### 6.1.1 概要

Linux上で動作する待受コマンドである。初期対応環境はWSL2とする。

任意のクライアントが接続するUnix Domain Socketを提供し、接続ごとにWindows側子コマンドを起動する。

### 6.1.2 主な責務

- Unix Domain Socketを作成する
- ソケットへの接続を受け付ける
- 接続ごとに`pipeferry.exe npipe-connect`を起動する
- Unix Domain Socketと子プロセスの標準入出力を双方向コピーする
- 接続終了時に子プロセスを終了する
- 停止時に待受ソケットと実行中の接続を閉じる
- 古いソケットファイルを安全に除去する
- 多重起動を防止する
- 状態確認と診断情報を提供する

## 6.2 `pipeferry.exe npipe-connect`

### 6.2.1 概要

Windows上で動作する短命な接続コマンドである。

WSL側リスナーの子プロセスとして起動され、指定されたWindows Named Pipeへ接続する。

### 6.2.2 主な責務

- 指定されたWindows Named Pipeへ接続する
- 標準入力からNamed Pipeへバイト列を転送する
- Named Pipeから標準出力へバイト列を転送する
- 片方向の終了やエラーを検出し、もう片方も確実に終了する
- 接続失敗を明確な終了コードと標準エラーで通知する
- 設定ストアや秘密情報へアクセスしない

## 7. 通信仕様

## 7.1 Unix socketクライアントとLinux側リスナー

通信方式はUnix Domain Socketのストリーム型とする。

クライアントから見た振る舞いは、通常のストリーム型Unix Domain Socketと同等でなければならない。

リスナーは上位プロトコルの長さヘッダー、メッセージ種別、文字コードを解析しない。SSH Agent用途では`SSH_AUTH_SOCK`として利用できること。

## 7.2 Linux側リスナーとWindows側子コマンド

通信には子プロセスの標準入力と標準出力を使用する。

| 方向 | 通信経路 |
|---|---|
| WSLからWindows | Unix Domain Socketから子プロセスの標準入力 |
| WindowsからWSL | 子プロセスの標準出力からUnix Domain Socket |
| ログとエラー | 子プロセスの標準エラー |

初期実装では独自ヘッダー、チャネルID、フレーム長、多重化プロトコルを使用しない。

## 7.3 Windows側コマンドとNamed Pipeサービス

通信方式はWindows Named Pipeの双方向バイトストリームとする。

Named Pipe名はコマンドライン引数または環境変数で明示的に指定する。

Windows OpenSSH Agentへ接続する場合は次を指定する。

```text
\\.\pipe\openssh-ssh-agent
```

OmniSSHAgentへ接続する場合は、次のような専用パイプ名も設定可能とする。

```text
\\.\pipe\omnisshagent
```

Windows側コマンドの再ビルドを行わず切り替えられる設計にする。

## 8. 機能要求

## 8.1 `pipeferry unix-listen`の機能要求

### WSL-FR-001 Unix Domain Socketの待ち受け

指定されたパスにストリーム型Unix Domain Socketを作成し、複数のクライアント接続を受け付けられること。

### WSL-FR-002 既定ソケットパス

既定パスは次の優先順位で決定すること。

1. コマンドライン引数`--socket`
2. 環境変数`PIPEFERRY_SOCKET`
3. `XDG_RUNTIME_DIR`が存在する場合は`$XDG_RUNTIME_DIR/pipeferry/pipeferry.sock`
4. それ以外は`$HOME/.local/run/pipeferry/pipeferry.sock`

### WSL-FR-003 ソケットディレクトリの作成

ソケットの親ディレクトリが存在しない場合は作成すること。

作成するディレクトリの権限は`0700`とする。

### WSL-FR-004 ソケット権限

作成したUnix Domain Socketの権限を`0600`とすること。

起動時に`umask 077`相当の保護を行うこと。

### WSL-FR-005 古いソケットの処理

起動時に同名パスが存在する場合、次の判定を行うこと。

1. 現在接続可能なら、既存プロセスが稼働中として新規起動を拒否する
2. 接続不能で、対象がUnix Domain Socketなら古いソケットとして削除する
3. 通常ファイルやディレクトリなら削除せず、エラー終了する

### WSL-FR-006 多重起動防止

同じソケットパスを利用するプロセスの多重起動を防止すること。

ソケット接続確認に加え、ロックファイルと`flock`を利用することを推奨する。

### WSL-FR-007 Windows側子コマンドの決定

接続ごとに起動する子コマンドは次の優先順位で決定すること。

1. コマンドライン引数`--exec`
2. 環境変数`PIPEFERRY_EXEC`

子コマンドは必須とし、未指定の場合は設定エラーとして終了すること。実行ファイルはWSLの`PATH`から探索できるほか、絶対パスも指定できること。実行できない場合は、指定されたコマンドと探索条件を標準エラーへ表示して終了すること。

### WSL-FR-008 接続ごとの子プロセス起動

Unix Domain Socketの接続を受け付けるたびに、`pipeferry.exe npipe-connect`を一つ起動すること。

一つのUnix接続と一つのWindows側コマンドを一対一で対応させること。

### WSL-FR-009 双方向コピー

次の二方向を同時にコピーすること。

- Unix Domain Socketから子プロセスの標準入力
- 子プロセスの標準出力からUnix Domain Socket

一方が正常終了、異常終了、キャンセルのいずれかになった場合、もう一方のコピー処理も終了させること。

### WSL-FR-010 子プロセスの標準エラー

子プロセスの標準エラーは、Linux側リスナーのログへ転送すること。

標準出力と混在させてはならない。

### WSL-FR-011 接続終了処理

Unix Domain Socketが切断された場合、次を確実に実行すること。

- 子プロセスの標準入力を閉じる
- 必要に応じて子プロセスを終了する
- 標準出力の読み取りを終了する
- 子プロセスを`Wait`して回収する
- 関連するゴルーチンを終了する

### WSL-FR-012 プロセス停止

`SIGINT`と`SIGTERM`を受信した場合、次の順に停止すること。

1. 新規接続の受け付けを停止
2. 待受ソケットを閉じる
3. 実行中の接続をキャンセル
4. 子プロセスを終了
5. ゴルーチンの終了を待機
6. Unix Domain Socketファイルを削除

### WSL-FR-013 並行接続

少なくとも32接続を同時に処理できること。

接続ごとの状態が他の接続へ影響しないこと。

### WSL-FR-014 状態確認

次の状態を確認できる`status`サブコマンドを提供すること。

- リスナーの起動状態
- Unix Domain Socketの存在
- Unix Domain Socketへの接続可否
- Windows側子コマンドの探索結果
- 現在の設定値

### WSL-FR-015 診断

`doctor`サブコマンドを提供し、次を診断できること。

- WSL相互運用機能でWindows実行ファイルを起動できるか
- Windows側コマンドが実行可能か
- Windows Named Pipeへ接続できるか
- SSH Agentへ鍵一覧要求を送信できるか
- ソケットディレクトリの権限が適切か

診断結果は、人が読める形式とJSON形式の両方を選択可能にすることが望ましい。

### WSL-FR-016 バージョン表示

`version`サブコマンドまたは`--version`で、バージョン、コミットID、ビルド日時、対象OS、対象アーキテクチャを表示すること。

## 8.2 `pipeferry.exe npipe-connect`の機能要求

### WIN-FR-001 Named Pipeへの接続

指定されたWindows Named Pipeへクライアントとして接続すること。

### WIN-FR-002 Named Pipeの指定

Named Pipeは次の優先順位で決定すること。

1. コマンドライン引数`--pipe`
2. 環境変数`PIPEFERRY_PIPE`

Named Pipe名は必須とし、未指定の場合は設定エラーとして終了すること。

`openssh-ssh-agent`のような短い名前と完全なパスの両方を受け付けること。

短い名前が指定された場合は`\\.\pipe\`を補完すること。

### WIN-FR-003 接続タイムアウト

Named Pipeへの接続にタイムアウトを設定できること。

既定値は5秒とする。

接続先Named Pipeサービスが停止中の場合に無期限で待機してはならない。

### WIN-FR-004 標準入力からNamed Pipeへの転送

標準入力から読み取ったすべてのバイトを、順序を保持したままNamed Pipeへ書き込むこと。

### WIN-FR-005 Named Pipeから標準出力への転送

Named Pipeから読み取ったすべてのバイトを、順序を保持したまま標準出力へ書き込むこと。

### WIN-FR-006 標準出力の保護

標準出力には転送データ以外を一切出力しないこと。

バージョン表示やヘルプ表示を除き、通常動作中のログはすべて標準エラーへ出力すること。

### WIN-FR-007 双方向コピーの終了制御

一方の転送が終了した場合、もう一方の転送を無期限に残さないこと。

Named Pipe、標準入力、標準出力を適切に閉じ、プロセスが終了できること。

### WIN-FR-008 親プロセス終了への追従

WSL側の親プロセスまたは標準入出力が終了した場合、Windows側コマンドも速やかに終了すること。

孤児プロセスとして残り続けないこと。

### WIN-FR-009 エラー通知

Named Pipeへの接続失敗、読み書き失敗、設定不正を標準エラーへ出力し、定義済みの終了コードで終了すること。

### WIN-FR-010 設定ストア非依存

Windows Credential Manager、特定サービスの設定ファイル、レジストリを参照しないこと。

接続先は引数と環境変数だけで決定すること。

### WIN-FR-011 バージョン表示

`--version`で、バージョン、コミットID、ビルド日時、対象OS、対象アーキテクチャを表示すること。

### WIN-FR-012 単体診断

`--check`を指定した場合、Named Pipeへ接続できるか確認して、データ転送を開始せず終了できること。

## 9. コマンドライン仕様

## 9.1 Linux側コマンド

### 起動例

```bash
pipeferry unix-listen
```

```bash
pipeferry unix-listen \
  --socket "$HOME/.local/run/pipeferry/pipeferry.sock" \
  --exec "/mnt/c/Users/user/AppData/Local/Programs/Pipeferry/pipeferry.exe npipe-connect --pipe openssh-ssh-agent"
```

### 初期サブコマンド

| コマンド | 目的 |
|---|---|
| `unix-listen` | Unix Domain Socketを待ち受ける |
| `ensure` | 未起動なら起動し、起動済みなら正常終了する |
| `status` | 稼働状態を表示する |
| `doctor` | WSLとWindows間の接続を診断する |
| `version` | バージョン情報を表示する |

### 初期オプション

| オプション | 内容 |
|---|---|
| `--socket` | Unix Domain Socketのパス |
| `--exec` | 接続ごとに起動する子コマンド |
| `--connect-timeout` | Named Pipe接続タイムアウト |
| `--shutdown-timeout` | 停止処理の最大待機時間 |
| `--log-level` | `error`、`warn`、`info`、`debug` |
| `--log-format` | `text`または`json` |
| `--log-file` | ログファイルのパス |
| `--foreground` | 前面で実行する |
| `--json` | 状態や診断結果をJSONで出力する |
| `--version` | バージョン情報を表示する |

`unix-listen`は前面実行を基本とする。

バックグラウンド起動は`ensure`または外部のプロセスマネージャーが担当する。

## 9.2 Windows側コマンド

### 起動例

```powershell
pipeferry.exe npipe-connect --pipe openssh-ssh-agent
```

### 初期オプション

| オプション | 内容 |
|---|---|
| `--pipe` | 接続するWindows Named Pipe名 |
| `--connect-timeout` | 接続タイムアウト |
| `--check` | 接続確認だけ行う |
| `--log-level` | 標準エラーへ出すログレベル |
| `--version` | バージョン情報を表示する |

## 10. 環境変数

| 環境変数 | 対象 | 内容 |
|---|---|---|
| `PIPEFERRY_SOCKET` | WSL | Unix Domain Socketのパス |
| `PIPEFERRY_EXEC` | WSL | 接続ごとに起動する子コマンド |
| `PIPEFERRY_PIPE` | Windows | Windows Named Pipe名 |
| `PIPEFERRY_CONNECT_TIMEOUT` | Windows | Named Pipe接続タイムアウト |
| `PIPEFERRY_LOG_LEVEL` | 両方 | ログレベル |
| `PIPEFERRY_LOG_FILE` | WSL | ログファイルのパス |

コマンドライン引数を環境変数より優先する。

## 11. 終了コード

終了コードは両プロセスで可能な限り共通化する。

| 終了コード | 意味 |
|---:|---|
| 0 | 正常終了 |
| 1 | 未分類の実行時エラー |
| 2 | コマンドラインまたは設定の不正 |
| 3 | 必要なファイルまたは実行ファイルが見つからない |
| 4 | Unix Domain Socketの作成または待受失敗 |
| 5 | Windows Named Pipeへの接続失敗 |
| 6 | データ転送エラー |
| 7 | 多重起動を検出 |
| 8 | タイムアウト |
| 9 | 診断失敗 |

通常のクライアント切断やEOFは、エラーとして扱わず終了コード0とする。

## 12. ログ仕様

### 12.1 ログへ含める情報

- プロセス起動と終了
- バージョン
- Unix Domain Socketのパス
- Windows側子コマンド
- Windows Named Pipe名
- 接続ID
- 子プロセスID
- 接続開始時刻と終了時刻
- 転送したバイト数
- 終了理由
- エラー種別
- タイムアウト

### 12.2 ログへ含めない情報

- 中継対象プロトコルの要求ペイロード
- 中継対象プロトコルの応答ペイロード
- 秘密鍵
- 署名対象データ
- パスフレーズ
- 環境変数全体
- 標準入力と標準出力の生データ

### 12.3 ログ出力先

`pipeferry.exe npipe-connect`は標準エラーだけを利用する。

`pipeferry unix-listen`は標準エラーを既定とし、必要に応じてログファイルへ出力できること。

ログファイルを利用する場合は、ユーザーだけが読み書きできる権限にすること。

## 13. セキュリティ要求

### SEC-001 ローカル通信限定

TCPやUDPの待受ポートを作成しないこと。

WSL内のUnix Domain SocketとWindows Named Pipeだけを利用すること。

### SEC-002 Unixソケットのアクセス制御

ソケットディレクトリを`0700`、ソケットを`0600`にし、同一Linuxユーザーだけが接続できること。

### SEC-003 Named Pipeのアクセス制御

Windows側のNamed Pipeサーバーで適切なACLを設定する責務は、接続先サービスが持つ。

Windows側コマンドは現在のユーザー権限で接続し、昇格を要求しないこと。

### SEC-004 データ非保存

両プロセスは中継したSSH Agentデータをファイル、標準エラー、イベントログへ保存しないこと。

### SEC-005 引数への秘密情報禁止

コマンドライン引数や環境変数に秘密鍵、パスフレーズ、署名データを渡さないこと。

### SEC-006 子プロセスの固定

Windows側子コマンドの探索結果が想定外の場所を指していないか、`doctor`で表示すること。

インストーラー連携後は、既定のインストール先を優先して探索する方式も検討する。

### SEC-007 データサイズ制限

中継処理はバイトストリームとして動作するため、メッセージ単位のメモリ確保を行わないこと。

`io.Copy`相当の固定サイズバッファを利用し、入力値に応じて巨大なメモリを確保しないこと。

## 14. 非機能要求

## 14.1 対応環境

初期リリースでは次を正式対象とする。

- Windows 11 x86-64
- WSL2
- Ubuntuのサポート対象版
- GoのCGOを必要としないLinuxバイナリ
- Windows側はネイティブWindowsバイナリ

ARM64対応はコード上で阻害しないが、初期受け入れ対象からは外してよい。

## 14.2 依存関係

### WSL側

可能な限りGo標準ライブラリだけで実装する。

`Socat`、PowerShell、Python、Node.jsを必要としないこと。

### Windows側

Go標準ライブラリに加え、Windows Named Pipe接続のため`github.com/Microsoft/go-winio`の利用を許可する。

特定のNamed Pipeサービスのパッケージへ依存しない、独立した小さなバイナリにすることが望ましい。

## 14.3 性能

- 32接続を同時に処理できること
- 接続ごとに不要な大容量バッファを確保しないこと
- アイドル状態でCPUを継続的に消費しないこと
- 中継対象の通信へ不必要な遅延を追加しないこと
- 100回の連続接続後にプロセス、ファイルディスクリプタ、ゴルーチンが増加し続けないこと

Windows側コマンド起動時間はベンチマークで測定する。

常駐多重化方式への移行は、実測で問題が確認された場合にだけ検討する。

## 14.4 信頼性

- クライアントの異常切断でリスナー全体が停止しないこと
- 一つの子プロセスの異常終了が他の接続へ影響しないこと
- Named Pipeサービス停止中に無期限で待機しないこと
- 再起動後に古いソケットファイルから復旧できること
- 停止後にWindows側コマンドが残留しないこと
- 終了処理でデッドロックしないこと

## 14.5 保守性

- OS固有コードをビルドタグで分離すること
- 転送処理をテスト可能な`io.Reader`と`io.Writer`ベースで設計すること
- グローバル変数を最小化すること
- `context.Context`でキャンセルを伝播すること
- 終了処理を一箇所へ集約すること
- ログと転送データの出力経路を型または構造上分離すること

## 15. 推奨プロジェクト構成

```text
cmd
└─ pipeferry
   └─ main.go

internal
├─ buildinfo
│  └─ buildinfo.go
├─ command
│  ├─ root.go
│  ├─ unix_listen_linux.go
│  ├─ npipe_connect_windows.go
│  ├─ status_linux.go
│  └─ doctor_linux.go
├─ streamcopy
│  ├─ copy.go
│  └─ copy_test.go
├─ execbridge
│  ├─ process_linux.go
│  └─ lifecycle_linux.go
├─ unixsocket
│  ├─ listener_linux.go
│  └─ stale_linux.go
└─ namedpipe
   ├─ client_windows.go
   └─ proxy_windows.go
```

`streamcopy`にはOSに依存しない双方向コピーと終了制御を置く。

`unixsocket`、`execbridge`、`namedpipe`はOS境界ごとに責務を分け、共通の転送処理だけを`streamcopy`へ置く。

## 16. 処理シーケンス

## 16.1 正常接続

```text
1. `pipeferry unix-listen`がUnix Domain Socketを待ち受ける
2. Unix socketクライアントが待受ソケットへ接続する
3. Linux側リスナーが接続をacceptする
4. Linux側リスナーが`pipeferry.exe npipe-connect`を起動する
5. Windows側コマンドがNamed Pipeへ接続する
6. Unix Socketからstdinへの転送を開始する
7. Named Pipeからstdoutへの転送を開始する
8. 双方向のバイトストリーム転送を継続する
9. Unix socketクライアントが接続を閉じる
10. Linux側リスナーがstdinを閉じる
11. Windows側コマンドがNamed Pipeを閉じて終了する
12. Linux側リスナーが子プロセスを回収して接続処理を終了する
```

## 16.2 Named Pipeサービス停止中

```text
1. Unix socketクライアントがUnix Domain Socketへ接続する
2. Linux側リスナーが`pipeferry.exe npipe-connect`を起動する
3. Windows側コマンドがNamed Pipe接続を試みる
4. 接続タイムアウトまたは接続失敗になる
5. Windows側コマンドが標準エラーへ理由を出力する
6. Windows側コマンドが終了コード5または8で終了する
7. Linux側リスナーがUnix接続を閉じる
8. SSHクライアントへ通信失敗が返る
9. Linux側リスナー本体は待受を継続する
```

## 16.3 Linux側リスナー停止

```text
1. SIGTERMを受信する
2. 新規acceptを停止する
3. すべての接続コンテキストをキャンセルする
4. Unix Domain Socketを閉じる
5. 子プロセスの標準入力を閉じる
6. 終了しない子プロセスを停止する
7. すべての処理の終了を待つ
8. ソケットファイルとロックファイルを削除する
9. 正常終了する
```

## 17. テスト要求

## 17.1 単体テスト

### 共通転送処理

- 一方向コピーが正常に完了する
- 双方向コピーが正常に完了する
- 片方向EOFで反対側が終了する
- コンテキストキャンセルで両方向が終了する
- 読み込みエラーを返せる
- 書き込みエラーを返せる
- ゴルーチンが残留しない

### WSL側

- ソケットパスの優先順位
- ブリッジパスの優先順位
- 古いソケットの判定
- 通常ファイルを誤削除しない
- 多重起動の検出
- 子プロセス終了コードの取り扱い
- シグナルによる終了

### Windows側

- Named Pipe名の正規化
- 接続タイムアウト
- 標準出力へログを出さない
- EOFを正常終了として扱う
- Named Pipe接続失敗時の終了コード

## 17.2 統合テスト

- テスト用Unix Domain Socketと子プロセス間で双方向通信できる
- テスト用Windows Named Pipeと標準入出力間で双方向通信できる
- バイナリデータが改変されない
- 大きなデータを複数回に分割して転送できる
- 部分読み込みと部分書き込みでも破損しない
- クライアント切断時に子プロセスが終了する
- 子プロセス異常終了時にUnix接続が閉じる

## 17.3 E2Eテスト

実際のWindows OpenSSH AgentまたはOmniSSHAgentを起動し、WSL2から次を確認する。

```bash
export SSH_AUTH_SOCK="$XDG_RUNTIME_DIR/pipeferry/pipeferry.sock"
ssh-add -l
ssh-add -L
ssh -T git@github.com
```

秘密鍵を必要としない自動テストでは、テスト専用のSSH Agentサーバーまたは一時鍵を利用する。

## 17.4 負荷と回帰テスト

- 100回の連続`ssh-add -l`
- 32並列の鍵一覧要求
- 接続途中でクライアントを強制終了
- 接続先Named Pipeサービスを通信中に停止
- Linux側リスナーを通信中に停止
- Windows側コマンドを通信中に強制終了
- WSL再起動後の古いソケット復旧
- 30分以上の待受後に正常接続

## 18. 受け入れ条件

初期実装は次の条件をすべて満たした場合に完了とする。

1. WSL2上で`pipeferry unix-listen`を起動できる
2. 指定パスに権限`0600`のUnix Domain Socketが作成される
3. テスト用Named Pipeサービスとの間で任意のバイナリデータを改変せず双方向転送できる
4. `SSH_AUTH_SOCK`を設定すると`ssh-add -l`が成功する
5. 接続先SSH Agentに登録された公開鍵を`ssh-add -L`で取得できる
6. SSH Agentユースケースで、実際のSSH署名を利用した認証が成功する
7. PowerShellを起動しない
8. `Socat`を必要としない
9. 独自多重化プロトコルを使用しない
10. 接続ごとに`pipeferry.exe npipe-connect`が起動し、接続終了後に残留しない
11. Named Pipeサービス停止中でもLinux側リスナー本体は停止しない
12. 32並列接続を処理できる
13. 100回の連続接続後にゴルーチン、プロセス、ファイルディスクリプタが増え続けない
14. Windows側コマンドの標準出力へログが混入しない
15. `SIGTERM`による終了でUnixソケットと子プロセスが残留しない
16. 単体テストと統合テストが自動実行できる
17. WindowsとLinuxのリリースバイナリを同じバージョン番号で生成できる


## 19. 将来拡張

初期実装の性能測定後、必要な場合に次を検討する。

- Windows側コマンドの常駐化
- Go対Goのバージョン付き多重化プロトコル
- Windows Job Objectによる子プロセス管理
- systemdユーザーサービスへの登録
- PowerShellワンライナーインストーラーの提供
- WindowsバイナリのAuthenticode署名
- 自動アップデート
- OmniSSHAgentなど個別サービス向け設定例の提供
- WSLディストリビューションごとの自動セットアップ
- ARM64ビルド

常駐多重化は、接続ごとのWindowsプロセス起動が実測上の問題になった場合だけ導入する。

## 20. 実装上の重要決定

| 項目 | 決定 |
|---|---|
| WSL側実装 | Goで新規実装 |
| Windows側実装 | Goで新規実装 |
| PowerShell | 使用しない |
| Socat | 使用しない |
| 接続モデル | Unix接続ごとにWindowsプロセスを一つ起動 |
| WSLとWindows間 | 標準入力と標準出力 |
| 多重化 | 初期実装では行わない |
| 上位プロトコル解析 | 行わない |
| Named Pipe | 必須指定。短い名前と完全パスを受け付ける |
| ログ | 標準エラー。標準出力は転送データ専用 |
| キャンセル | `context.Context`で伝播 |
| 追加ランタイム | 不要 |
| Linuxビルド | CGOなしを基本とする |

## 21. 完成後の利用イメージ

```bash
pipeferry ensure \
  --socket "${XDG_RUNTIME_DIR:-$HOME/.local/run}/pipeferry/ssh-agent.sock" \
  --exec "pipeferry.exe npipe-connect --pipe openssh-ssh-agent"

export SSH_AUTH_SOCK="${XDG_RUNTIME_DIR:-$HOME/.local/run}/pipeferry/ssh-agent.sock"
ssh-add -l
```

利用者はPowerShellスクリプトや`Socat`の存在を意識せず、Unix Domain SocketとWindows Named Pipeを汎用的に接続できることを最終的な目標とする。SSH Agent用途では、一般的なLinuxのSSH Agentと同じように`SSH_AUTH_SOCK`を設定して利用できること。
