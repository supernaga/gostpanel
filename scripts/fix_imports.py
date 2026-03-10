#!/usr/bin/env python3
"""
修复拆分后文件的导入问题
"""

import re
import os

# 需要修复的文件列表
FILES = [
    'internal/api/agent_handlers.go',
    'internal/api/client_handlers.go',
    'internal/api/clone_handlers.go',
    'internal/api/config_version_handlers.go',
    'internal/api/node_group_handlers.go',
    'internal/api/node_handlers.go',
    'internal/api/notification_handlers.go',
    'internal/api/plan_handlers.go',
    'internal/api/port_forward_handlers.go',
    'internal/api/proxy_chain_handlers.go',
    'internal/api/rule_handlers.go',
    'internal/api/site_config_handlers.go',
    'internal/api/system_handlers.go',
    'internal/api/tag_handlers.go',
    'internal/api/traffic_handlers.go',
    'internal/api/tunnel_handlers.go',
    'internal/api/user_handlers.go',
]

# 最小化的导入模板
MINIMAL_IMPORTS = '''import (
	"fmt"
	"net/http"
	"strconv"

	"github.com/supernaga/gost-panel/internal/model"
	"github.com/gin-gonic/gin"
)'''

def fix_file_imports(filepath):
    """修复单个文件的导入"""
    with open(filepath, 'r', encoding='utf-8') as f:
        content = f.read()

    # 找到 package 声明和第一个 import 之间的内容
    lines = content.split('\n')

    # 找到 package 行
    package_line_idx = -1
    for i, line in enumerate(lines):
        if line.startswith('package '):
            package_line_idx = i
            break

    if package_line_idx == -1:
        print(f'[SKIP] {filepath} - 找不到 package 声明')
        return

    # 找到第一个非注释、非空行（import 或代码开始）
    import_start = -1
    import_end = -1
    in_import = False

    for i in range(package_line_idx + 1, len(lines)):
        line = lines[i].strip()

        if line.startswith('import'):
            import_start = i
            in_import = True
            if '(' in line:
                continue
            else:
                import_end = i
                break
        elif in_import:
            if ')' in line:
                import_end = i
                break
        elif line and not line.startswith('//'):
            # 找到第一个代码行
            break

    if import_start == -1:
        print(f'[SKIP] {filepath} - 找不到 import')
        return

    # 提取文件内容，分析实际需要的导入
    file_content = '\n'.join(lines[import_end+1:])

    # 检测需要的导入
    needed_imports = set()

    # 标准库
    if 'fmt.' in file_content or 'fmt.Sprintf' in file_content or 'fmt.Errorf' in file_content:
        needed_imports.add('fmt')
    if 'strconv.' in file_content:
        needed_imports.add('strconv')
    if 'strings.' in file_content:
        needed_imports.add('strings')
    if 'time.' in file_content or 'time.Now' in file_content:
        needed_imports.add('time')
    if 'net.' in file_content or 'net.SplitHostPort' in file_content:
        needed_imports.add('net')
    if 'http.' in file_content or 'http.Status' in file_content:
        needed_imports.add('net/http')
    if 'json.' in file_content or 'json.Unmarshal' in file_content:
        needed_imports.add('encoding/json')
    if 'yaml.' in file_content or 'yaml.Marshal' in file_content:
        needed_imports.add('github.com/goccy/go-yaml')
    if 'io.' in file_content:
        needed_imports.add('io')
    if 'os.' in file_content:
        needed_imports.add('os')
    if 'filepath.' in file_content:
        needed_imports.add('path/filepath')
    if 'sync.' in file_content:
        needed_imports.add('sync')

    # 项目内部包
    if 'model.' in file_content:
        needed_imports.add('github.com/supernaga/gost-panel/internal/model')
    if 'service.' in file_content:
        needed_imports.add('github.com/supernaga/gost-panel/internal/service')
    if 'gost.' in file_content or 'gost.NewConfigGenerator' in file_content:
        needed_imports.add('github.com/supernaga/gost-panel/internal/gost')
    if 'gin.' in file_content or '*gin.Context' in file_content:
        needed_imports.add('github.com/gin-gonic/gin')

    # 构建新的导入块
    std_imports = sorted([imp for imp in needed_imports if '/' not in imp or imp.startswith('encoding/') or imp.startswith('net/') or imp.startswith('path/')])
    external_imports = sorted([imp for imp in needed_imports if '/' in imp and not imp.startswith('encoding/') and not imp.startswith('net/') and not imp.startswith('path/')])

    new_import_lines = ['import (']

    if std_imports:
        for imp in std_imports:
            new_import_lines.append(f'\t"{imp}"')

    if std_imports and external_imports:
        new_import_lines.append('')

    if external_imports:
        for imp in external_imports:
            new_import_lines.append(f'\t"{imp}"')

    new_import_lines.append(')')

    # 重新组装文件
    new_lines = (
        lines[:package_line_idx+1] +
        [''] +
        new_import_lines +
        [''] +
        lines[import_end+1:]
    )

    # 写回文件
    with open(filepath, 'w', encoding='utf-8') as f:
        f.write('\n'.join(new_lines))

    print(f'[OK] {filepath} - 已修复导入')

def main():
    print('开始修复导入...\n')

    for filepath in FILES:
        if os.path.exists(filepath):
            fix_file_imports(filepath)
        else:
            print(f'[SKIP] {filepath} - 文件不存在')

    print('\n完成！')

if __name__ == '__main__':
    main()
