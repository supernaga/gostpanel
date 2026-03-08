import { driver } from 'driver.js'
import 'driver.js/dist/driver.css'

// 仪表盘引导
export const dashboardGuide = () => {
  const driverObj = driver({
    showProgress: true,
    animate: true,
    nextBtnText: '下一步',
    prevBtnText: '上一步',
    doneBtnText: '完成',
    steps: [
      {
        element: '.dashboard',
        popover: {
          title: '欢迎使用 GOST Panel',
          description: '这是您的管理仪表盘，可以查看系统概览和实时统计信息。',
          side: 'bottom',
          align: 'center',
        }
      },
      {
        element: '.n-layout-sider',
        popover: {
          title: '导航菜单',
          description: '通过侧边栏菜单可以访问不同的管理功能：节点、客户端、隧道、规则等。',
          side: 'right',
          align: 'start',
        }
      },
      {
        element: '.header-actions',
        popover: {
          title: '快捷操作',
          description: '这里可以全局搜索、切换主题、切换语言和管理账户。',
          side: 'bottom',
          align: 'end',
        }
      },
    ]
  })
  driverObj.drive()
  return driverObj
}

// 节点管理引导
export const nodeGuide = () => {
  const driverObj = driver({
    showProgress: true,
    animate: true,
    nextBtnText: '下一步',
    prevBtnText: '上一步',
    doneBtnText: '完成',
    steps: [
      {
        element: '#create-node-btn',
        popover: {
          title: '创建节点',
          description: '点击此按钮创建您的第一个代理节点。节点是运行 GOST 代理服务的服务器。',
          side: 'bottom',
          align: 'end',
        }
      },
    ]
  })
  driverObj.drive()
  return driverObj
}

// 检查是否需要显示引导
export const shouldShowGuide = (guideKey: string): boolean => {
  return !localStorage.getItem(`guide_${guideKey}`)
}

// 标记引导已完成
export const markGuideComplete = (guideKey: string) => {
  localStorage.setItem(`guide_${guideKey}`, 'true')
}

// 重置所有引导
export const resetAllGuides = () => {
  const keys = Object.keys(localStorage).filter(k => k.startsWith('guide_'))
  keys.forEach(k => localStorage.removeItem(k))
}
