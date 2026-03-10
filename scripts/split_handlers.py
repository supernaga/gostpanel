#!/usr/bin/env python3
"""
自动拆分 handlers.go 文件的脚本
"""

import re
import os

# 定义模块分割规则
MODULES = {
    'node_handlers.go': {
        'start': 82,
        'end': 943,
        'description': '节点管理模块',
        'includes_batch': True,  # 包含批量操作
    },
    'client_handlers.go': {
        'start': 943,
        'end': 1340,
        'description': '客户端管理模块',
    },
    'agent_handlers.go': {
        'start': 1340,
        'end': 1567,
        'description': 'Agent 接口模块',
    },
    'user_handlers.go': {
        'start': 1567,
        'end': 1945,
        'description': '用户管理模块（包含 2FA）',
    },
    'traffic_handlers.go': {
        'start': 1945,
        'end': 1981,
        'description': '流量历史模块',
    },
    'notification_handlers.go': {
        'start': 1981,
        'end': 2428,
        'description': '通知和告警模块',
    },
    'port_forward_handlers.go': {
        'start': 2428,
        'end': 2737,
        'description': '端口转发模块',
    },
    'node_group_handlers.go': {
        'start': 2737,
        'end': 3010,
        'description': '节点组（负载均衡）模块',
    },
    'system_handlers.go': {
        'start': 3010,
        'end': 3600,
        'description': '系统管理模块（日志、导出、备份）',
    },
    'proxy_chain_handlers.go': {
        'start': 3600,
        'end': 3828,
        'description': '代理链模块',
    },
    'tunnel_handlers.go': {
        'start': 3828,
        'end': 4088,
        'description': '隧道转发模块',
    },
    'site_config_handlers.go': {
        'start': 4088,
        'end': 4137,
        'description': '网站配置模块',
    },
    'tag_handlers.go': {
        'start': 4137,
        'end': 4433,
        'description': '标签管理模块',
    },
    'plan_handlers.go': {
        'start': 4433,
        'end': 4677,
        'description': '套餐管理模块',
    },
    'rule_handlers.go': {
        'start': 4677,
        'end': 5202,
        'description': '规则管理模块（Bypass、Admission、HostMapping 等）',
    },
    'clone_handlers.go': {
        'start': 5202,
        'end': 5390,
        'description': '克隆操作模块',
    },
    'config_version_handlers.go': {
        'start': 5390,
        'end': 5547,
        'description': '配置版本历史模块',
    },
}

def read_handlers_file():
    """读取原始 handlers.go 文件"""
    with open('internal/api/handlers.go', 'r', encoding='utf-8') as f:
        return f.readlines()

def extract_imports():
    """提取 import 部分"""
    lines = read_handlers_file()
    imports = []
    in_import = False
    for line in lines:
        if line.strip().startswith('import'):
            in_import = True
        if in_import:
            imports.append(line)
            if line.strip() == ')':
                break
    return ''.join(imports)

def create_module_file(filename, start_line, end_line, description):
    """创建模块文件"""
    lines = read_handlers_file()
    imports = extract_imports()

    # 提取指定行范围的内容
    content_lines = lines[start_line-1:end_line-1]

    # 生成文件头
    header = f"""// {description}
package api

{imports}

"""

    # 写入文件
    output_path = f'internal/api/{filename}'
    with open(output_path, 'w', encoding='utf-8') as f:
        f.write(header)
        f.writelines(content_lines)

    print(f'[OK] 创建 {filename} ({end_line - start_line} 行)')

def main():
    print('开始拆分 handlers.go 文件...\n')

    for filename, config in MODULES.items():
        create_module_file(
            filename,
            config['start'],
            config['end'],
            config['description']
        )

    print(f'\n[OK] 完成！共创建 {len(MODULES)} 个模块文件')
    print('\n下一步：')
    print('1. 检查生成的文件是否正确')
    print('2. 删除或清理原 handlers.go 文件')
    print('3. 运行 go build 测试编译')

if __name__ == '__main__':
    main()
