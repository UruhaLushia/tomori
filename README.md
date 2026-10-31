# Tomori

用 Go 编写的跨平台 Minecraft 启动器，基于 [mygo](https://github.com/egoist/mygo) 原生绘制界面。

## 特性

- **账户管理**：支持微软正版、第三方认证（Yggdrasil / authlib-injector）与离线账户，支持微软皮肤与披风管理。
- **实例管理**：兼容标准 `.minecraft` 与 Prism Launcher / MultiMC 实例；支持版本隔离、独立配置、组件更换（Minecraft / Fabric / Quilt / Forge / NeoForge）、实例复制与快捷方式创建。
- **资源与整合包**：内置 Modrinth 与 CurseForge 资源中心，支持模组（含中文译名检索）、资源包、光影、世界数据包与整合包的下载、前置解析、批量更新及多格式导入导出（MRPACK / CurseForge / MCBBS / Prism）。
- **内容与工具**：管理世界存档与备份、截图画廊、服务器列表（实时状态与快速加入），内置实时日志与崩溃诊断。
- **界面与网络**：支持深浅色主题、自定义强调色、多种导航布局、界面缩放与中英双语；支持 BMCLAPI 镜像、分片下载、限速与网络代理。

## 构建与运行

```sh
go run ./cmd/tomori

# Windows GUI
go tool mygo build -platform windows/amd64 -skip-installer

# Windows CLI（mygo 不接受 -tags/-ldflags，使用环境变量传递）
GOFLAGS='-tags=no_gui,no_tui,no_sqlite' MYGO_LDFLAGS='-H=windows' \
  go tool mygo build -platform windows/amd64 -skip-installer

# 全部桌面平台
go tool mygo build -platform windows/amd64,windows/arm64,linux/amd64,linux/arm64,linux/loong64,darwin/amd64,darwin/arm64 -skip-installer -skip-dmg
```

Windows CLI 需要单独构建：mygo 默认给 Windows 追加 `-H=windowsgui`，程序不会附着控制台，因此从终端启动时看不到输出和错误。CLI 构建用 `MYGO_LDFLAGS='-H=windows'` 切换回控制台子系统，并可用 `no_gui`、`no_tui` 去掉 GUI 和交互式终端入口，不能直接复用 GUI 产物。

支持的自定义构建标签：

- `no_gui`：移除桌面 GUI 入口，仅保留命令行入口。
- `no_tui`：移除交互式终端界面，适合脚本或无终端环境。
- `no_sqlite`：不编译 SQLite 存储，改用 JSON 配置存储。

多个标签用逗号连接，例如上面的 Windows CLI 构建同时使用了 `no_gui,no_tui,no_sqlite`。平台标签（如 `windows`、`linux`、`darwin`）由 Go 和 mygo 根据目标平台自动设置，不需要手动传入。

Windows 默认构建使用 GUI 子系统；需要自定义标签或链接参数时，分别设置 `GOFLAGS` 和 `MYGO_LDFLAGS`。

```sh
tomori instances list [--json]
tomori launch <实例名称/路径/key>
tomori open <实例名称/路径/key>
```

## 致谢

- [mygo](https://github.com/egoist/mygo)：原生跨平台 UI 框架
- [piko](https://github.com/UruhaLushia/piko)：大文件分片下载
- [Modrinth](https://modrinth.com) 与 [CurseForge](https://www.curseforge.com/minecraft)：模组、整合包与资源在线目录支持
- [HMCL](https://github.com/HMCL-dev/HMCL) 与 [MC 百科](https://www.mcmod.cn/)：模组中文译名数据与 MCBBS 格式参考
- [BMCLAPI](https://bmclapi2.bangbang93.com)：国内下载加速镜像（bangbang93）
- [Mojang](https://www.minecraft.net)：版本清单与官方游戏资源

## 许可

本项目采用 [GNU General Public License v3.0](LICENSE)。
