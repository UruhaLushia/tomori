# Tomori

一个用 Go 编写的 Minecraft Java 版启动器，界面由 [mygo](https://github.com/egoist/mygo) 原生绘制，不依赖 WebView。

项目还在开发中，这里先放一份介绍。源码和安装包暂未发布。

## 目前在做

- 原版游戏的下载、安装、校验与启动
- 多账户管理、离线账户与微软登录
- 本机 Java 检测与官方运行时自动下载
- 并行下载，大文件使用 [piko](https://github.com/UruhaLushia/piko) 分片加速
- 版本隔离、窗口状态保存，以及 SQLite / JSON 配置切换

微软登录流程已接入，Minecraft API 访问还需审核通过，暂时无法完成登录。

## About

Tomori is a native desktop launcher for Minecraft: Java Edition, built with Go and mygo. It handles game installation, Java runtimes, and player accounts.

The project is still in development. This repository is an introduction to Tomori; source code and downloads are not available yet.

Microsoft sign-in uses the official device-code flow to verify Java Edition ownership and load the player's profile. Minecraft API access still requires approval.

Application (client) ID: `78432422-b913-4a45-ba48-3f6699f1e6f3`

## Notice

Tomori is not an official Minecraft product and is not affiliated with Mojang Studios or Microsoft.

[Minecraft EULA](https://www.minecraft.net/eula) · [Usage Guidelines](https://www.minecraft.net/usage-guidelines)
