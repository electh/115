<p align="center">
  <img width="120" height="120" alt="91" src="https://github.com/user-attachments/assets/5b323c94-bbd3-4dce-bbc8-adc86935b7de" />
</p>

<p align="center">
  😄 个人私有视频站 😄
</p>

## 功能特性

- **多网盘接入** — 支持115、PikPak、123网盘、联通网盘、光鸭网盘、OneDrive、Google Drive、WebDAV等
- **低带宽播放** — 115、PikPak、123网盘、联通网盘、光鸭网盘、OneDrive 支持302模式，播放视频不消耗带宽
- **短视频模式** — 一键切换抖音风格，沉浸刷片
- **视频分享** — 视频支持一次性分享，"看完即焚"
- **爬虫脚本** — 支持导入自定义脚本，但是有一些规范，具体可以参考 [SpiderFor91](https://github.com/Just-Spider/SpiderFor91)
- **接入TG** — 支持通过机器人接入 Telegram，具体可查看 [接入指南](backend/docs/Enable_Telegram.md)
- **网络封面** — 支持通过 MetaTube 匹配文件名编号或标题，获取封面；找不到或抓取失败时自动回退视频抽帧

## 预览图
<img src="ReadMeImage/home.webp" alt="首页展示" width="100%" />
<img src="ReadMeImage/player.webp" alt="视频播放页展示" width="100%" />
<img src="ReadMeImage/admin.webp" alt="后台展示" width="100%" />

## 快速开始

### 方式一：一键安装脚本（推荐）

```bash
sudo apt update && sudo apt install -y curl ca-certificates
curl -fsSL https://raw.githubusercontent.com/electh/115/main/install.sh -o install.sh
sudo env GITHUB_REPO=electh/115 bash install.sh
```
部署完成后访问：`http://服务器IP:9191/`

安装后自动注册 `91` 管理命令：
```bash
sudo 91                  # 打开管理菜单
sudo 91 stop             # 停止服务
sudo 91 restart          # 重启服务
sudo 91 update           # 更新到本仓库最新版本
sudo 91 status           # 查看运行状态
sudo 91 reset-password   # 重置密码
```

默认安装目录为 `/opt/video-site-91`，服务名为 `video-site-91`，管理命令仍为 `91`。可通过环境变量指定版本、安装目录和端口，例如：

```bash
sudo env GITHUB_REPO=electh/115 VERSION=v0.3.7 INSTALL_PATH=/opt/video-site-91 FRONTEND_PORT=9191 bash install.sh
```

若已安装上游版本，更新时显式指定本仓库：`sudo env GITHUB_REPO=electh/115 91 update`。

使用自定义安装目录时，后续管理命令也需传入相同的 `INSTALL_PATH`，例如 `sudo env INSTALL_PATH=/srv/video-site 91 restart`。

### 方式二：Docker Compose 部署

**1. 准备目录**
```bash
mkdir video-site-91 && cd video-site-91
```
**2. 拉取仓库内置`docker-compose.yml`**
```bash
curl -fsSL https://raw.githubusercontent.com/electh/115/main/docker-compose.yml -o docker-compose.yml
```
**3. 启动**
```bash
docker compose up -d
```

模板使用本仓库镜像 `ghcr.io/electh/115:stable`，支持 amd64 和 arm64。需要固定版本时，将 `image` 改为 `ghcr.io/electh/115:v0.3.7`。默认访问 `http://服务器IP:9191/`；修改宿主机端口时调整 `ports`，例如 `8080:9191`。

**常用命令：**
```bash
docker compose pull && docker compose up -d             # 更新并重启
docker exec -it video-site-91 /opt/video-site-91/server reset-password # 重置密码
docker compose logs -f                                  # 查看日志
```

已有上游 Compose 部署需先将网站服务的 `image` 改为 `ghcr.io/electh/115:stable`，并将 `VIDEO_GITHUB_REPO`（若已设置）改为 `electh/115`，再执行更新命令；沿用原有端口和数据挂载配置。

## 数据默认存放位置

以下是默认配置下的路径。Compose 的 `./data` 位于 `docker-compose.yml` 所在目录，不要求部署在 `/root`。

| 内容 | 一键脚本部署 | Docker Compose 宿主机 | Docker 容器内 |
| --- | --- | --- | --- |
| 运行配置 | `/opt/video-site-91/config.yaml` | `./data/config.yaml` | `/opt/video-site-91/data/config.yaml` |
| 数据库 | `/opt/video-site-91/data/video-site.db` | `./data/video-site.db` | `/opt/video-site-91/data/video-site.db` |
| 封面、预览片段 | `/opt/video-site-91/data/previews/` | `./data/previews/` | `/opt/video-site-91/data/previews/` |
| 手动上传的视频 | `/opt/video-site-91/data/uploads/` | `./data/uploads/` | `/opt/video-site-91/data/uploads/` |
| Telegram 视频（启用后） | `/opt/video-site-91/data/telegram/library/` | `./data/telegram/library/` | `/var/lib/telegram-bot-api/library/`（使用 TG Compose 模板） |

安装时修改 `INSTALL_PATH` 后，一键脚本部署路径随安装目录变化。修改 `storage.data_dir` 后，普通媒体目录随之变化；`storage.db_dir` 留空时数据库跟随数据目录，填写后使用指定目录。相对路径按服务工作目录解析。Telegram 视频目录由 Telegram 部署文件的共享挂载决定，详见[接入指南](backend/docs/Enable_Telegram.md)。

数据库：存放视频信息、账号、密码、网盘凭证、视频的播放/点赞统计等

其他数据：包含封面图和预览片段、手动上传的视频、爬虫下载的视频、备份包等

## 配置与网络封面

在管理后台的配置面板编辑运行配置，或修改上表中的 `config.yaml`。完整配置项见 [config.example.yaml](backend/config.example.yaml)，重启生效和热更新范围在模板注释中注明。

使用网络封面需先部署独立的 MetaTube 服务，再添加以下配置并重启本站：

```yaml
metatube:
  url: "http://你的MetaTube服务地址:8080"
  token: ""                  # MetaTube 启用鉴权时填写
  timeout_seconds: 15
```

服务地址必须能从本站后端访问；Docker 容器内的 `localhost` 指向本站容器自身。地址留空时继续使用视频抽帧。网络匹配只处理缺少封面的视频，已有封面不自动替换。详细行为见[后端说明](backend/README.md#4-异步生成)。

## 其他说明

### 短视频模式
> ios设备不建议使用短视频模式

### 分享链接
> 视频支持生成分享链接，链接只能打开一次，链接分享的视频无需登录即可播放

<img src="ReadMeImage/share.webp" alt="分享页面展示" width="100%" />

### 三屏画面
> 只有竖屏视频支持三屏画面，只有电脑端支持三屏画面，三屏画面播放视频走的是服务器代理

<table>
  <tr>
    <td width="50%"><img src="ReadMeImage/single-screen.webp" alt="单个画面展示" width="100%" /></td>
    <td width="50%"><img src="ReadMeImage/triple-screen.webp" alt="三屏画面展示" width="100%" /></td>
  </tr>
  <tr>
    <td align="center">单屏画面</td>
    <td align="center">三屏画面</td>
  </tr>
</table>

## 使用须知

- **本项目仅面向个人私有部署**
- **请遵守法律法规**

## 致谢

- [Cli-Proxy-API-Management-Center](https://github.com/router-for-me/Cli-Proxy-API-Management-Center) — 参考其页面设计
- [ArtPlayer](https://github.com/zhw2590582/ArtPlayer) — 当前项目使用的视频播放器
- [OpenList](https://github.com/OpenListTeam/OpenList) — 参考其网盘接口

## 捐赠

如果这个项目对你有帮助，欢迎请我喝杯咖啡

<table>
  <tr>
    <td width="50%"><img src="ReadMeImage/donate-wechat.webp" alt="微信" width="100%" /></td>
    <td width="50%"><img src="ReadMeImage/donate-alipay.webp" alt="支付宝" width="100%" /></td>
  </tr>
  <tr>
    <td align="center">微信</td>
    <td align="center">支付宝</td>
  </tr>
</table>

## 赞助者

感谢支持，你的支持是我前进的动力💗

<table>
  <tr>
    <td align="center" width="120">
      <img src="ReadMeImage/sponsors/sponsor001.png" alt="*帅的头像" width="80" height="80" /><br />
      <sub>*帅</sub>
    </td>
  </tr>
</table>
