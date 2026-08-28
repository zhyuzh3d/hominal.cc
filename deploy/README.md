# Deployment

本目录保存可重复构建和注入 Hominal 身体包的部署定义。

开发同步与正式创生采用两条路径：开发阶段可以使用 Mutagen；正式创生从冻结版本生成明确的身体包，停止整仓实时双向同步，在纯净 Ubuntu 身体中启动该代实例。结束后由 Genesis Lab 读取和归档最终身体，身体变化不会自动覆盖下一代源代码。

正式 bundle 同时冻结 `hominal.service`、`hominal-launcher`、`hominal-generation-stop`、`hominal-persist-app-state` 和 Chrome 图形会话启动项。启动器把本代 `body/bin` 加入 PATH；到时停止器在 T0 计划截止处撤下活动实例标记并优雅停止服务；Chrome 启动项让同一持久 profile 在开机进入桌面后以本机 CDP 模式打开，供 `hominal-browser` 连接现有真实窗口。应用状态工具负责核验 Clash Verge 的配置入口仍指向 agent 持久卷。

## 微信桌面启动

当前 Linux 微信的真实启动链为：LightDM 自动进入 Xubuntu 桌面，XFCE 启动微信，辅助程序识别保存账号页面并触发官方登录请求，随后等待用户在手机微信中确认。手机确认由微信安全机制管理，项目保留这一真实外部交互。

目标设备首次初始化或系统恢复后执行：

```bash
sudo ./deploy/install-wechat-autostart.sh hominal
```

安装内容包括：

- `/etc/lightdm/lightdm.conf.d/50-hominal-autologin.conf`；
- `/home/hominal/.config/autostart/wechat.desktop`；
- `/home/hominal/.local/bin/wechat-autologin`；
- `/agent/state/logs/wechat-autologin.log`。

微信账号配置和文件分别位于 `/agent/state/profiles/wechat/config` 与 `/agent/state/profiles/wechat/files`。辅助程序只对保存账号的小型登录窗口发起一次登录请求；主界面已经存在时不产生输入。等待手机确认超过 30 秒会留下 `phone_confirmation_pending`，确认完成后记录 `login_completed`。
