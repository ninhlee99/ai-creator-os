// FlowCanvas - Interactive Node Graph Workflow Canvas (n8n & ComfyUI style)
(function () {
  'use strict';

  class FlowCanvas {
    constructor() {
      this.container = document.getElementById('flow-canvas-container');
      this.viewport = document.getElementById('flow-canvas-viewport');
      this.svgLayer = document.getElementById('flow-wires-svg');
      this.nodesLayer = document.getElementById('flow-nodes-layer');
      this.inspector = document.getElementById('flow-inspector');
      this.nodeCountBadge = document.getElementById('wf-node-count-badge');
      this.statusPill = document.getElementById('wf-status-pill');

      this.graph = window.__INITIAL_GRAPH__ || { id: 'custom_flow', name: 'Luồng tùy chỉnh', nodes: [], edges: [] };
      if (!this.graph.nodes) this.graph.nodes = [];
      if (!this.graph.edges) this.graph.edges = [];

      this.definitions = {};
      (window.__NODE_DEFINITIONS__ || []).forEach(d => {
        this.definitions[d.type] = d;
      });

      this.scale = 1.0;
      this.panX = 40;
      this.panY = 40;
      this.isPanning = false;
      this.panStartX = 0;
      this.panStartY = 0;

      this.selectedNodeId = null;
      this.activeConnecting = null; // { nodeId, portId, isOutput, startX, startY }
      this.tempWire = null;
      this.pendingConnect = null; // { fromNode, fromPort } for clicking '+' handle

      this.currentRunId = null;
      this.pollInterval = null;

      this.initEvents();
      this.render();
      this.bindToolbar();
      this.bindInspector();
      this.bindModal();
      this.updateZoomDisplay();
    }

    initEvents() {
      // Pan canvas by dragging background
      this.container.addEventListener('mousedown', (e) => {
        if (e.target === this.container || e.target === this.viewport || e.target === this.svgLayer) {
          this.isPanning = true;
          this.panStartX = e.clientX - this.panX;
          this.panStartY = e.clientY - this.panY;
          this.container.style.cursor = 'grabbing';
          this.deselectAll();
        }
      });

      window.addEventListener('mousemove', (e) => {
        if (this.isPanning) {
          this.panX = e.clientX - this.panStartX;
          this.panY = e.clientY - this.panStartY;
          this.applyTransform();
        } else if (this.activeConnecting) {
          this.updateTempWire(e.clientX, e.clientY);
        }
      });

      window.addEventListener('mouseup', () => {
        if (this.isPanning) {
          this.isPanning = false;
          this.container.style.cursor = 'default';
        }
        if (this.activeConnecting) {
          this.cancelConnecting();
        }
      });

      // Zoom via mouse wheel toward mouse point
      this.container.addEventListener('wheel', (e) => {
        e.preventDefault();
        const zoomFactor = 1.1;
        const rect = this.container.getBoundingClientRect();
        const mouseX = e.clientX - rect.left;
        const mouseY = e.clientY - rect.top;

        const prevScale = this.scale;
        if (e.deltaY < 0) {
          this.scale = Math.min(2.5, this.scale * zoomFactor);
        } else {
          this.scale = Math.max(0.35, this.scale / zoomFactor);
        }

        this.panX = mouseX - (mouseX - this.panX) * (this.scale / prevScale);
        this.panY = mouseY - (mouseY - this.panY) * (this.scale / prevScale);

        this.applyTransform();
        this.updateZoomDisplay();
      }, { passive: false });

      window.addEventListener('resize', () => {
        this.renderWires();
      });

      // Keyboard shortcuts
      window.addEventListener('keydown', (e) => {
        if ((e.key === 'Delete' || e.key === 'Backspace') && this.selectedNodeId) {
          const activeTag = document.activeElement ? document.activeElement.tagName.toLowerCase() : '';
          if (activeTag === 'input' || activeTag === 'textarea' || activeTag === 'select') return;
          this.deleteNode(this.selectedNodeId);
        } else if (e.key === '+' && !['input', 'textarea'].includes(document.activeElement?.tagName.toLowerCase())) {
          this.openAddNodeModal();
        } else if (e.key === 'Escape') {
          this.closeAddNodeModal();
          this.cancelConnecting();
        }
      });
    }

    applyTransform() {
      this.viewport.style.transform = `translate(${this.panX}px, ${this.panY}px) scale(${this.scale})`;
    }

    updateZoomDisplay() {
      const zoomText = document.getElementById('zoom-level');
      if (zoomText) {
        zoomText.innerText = `${Math.round(this.scale * 100)}%`;
      }
    }

    render() {
      this.nodesLayer.innerHTML = '';
      if (this.graph.nodes && this.graph.nodes.length > 0) {
        this.graph.nodes.forEach(node => {
          this.renderNode(node);
        });
      }
      this.renderWires();
      if (this.nodeCountBadge) {
        this.nodeCountBadge.innerText = `${(this.graph.nodes || []).length} nodes`;
      }
    }

    renderNode(node) {
      const def = this.definitions[node.type] || {
        type: node.type,
        title: node.title || node.type,
        category: node.category || 'process',
        inputs: [],
        outputs: []
      };

      const cat = node.category || def.category || 'process';

      const nodeEl = document.createElement('div');
      nodeEl.className = `wf-node cat-${cat}`;
      nodeEl.id = `node-${node.id}`;
      nodeEl.dataset.nodeId = node.id;
      nodeEl.style.left = `${node.x || 100}px`;
      nodeEl.style.top = `${node.y || 100}px`;

      if (this.selectedNodeId === node.id) {
        nodeEl.classList.add('selected');
      }

      // 1. Node Header Main (n8n Card layout)
      const header = document.createElement('div');
      header.className = 'wf-node-main';
      header.innerHTML = `
        <div class="wf-node-icon cat-${cat}">
          <span>${this.getCategoryIcon(cat)}</span>
        </div>
        <div class="wf-node-info">
          <div class="wf-node-title" title="${node.title || def.title || node.type}">
            ${node.title || def.title || node.type}
          </div>
          <div class="wf-node-subtitle" title="${def.title || node.type}">
            ${def.title || node.type}
          </div>
        </div>
        <div class="wf-node-actions">
          <button type="button" class="wf-node-del-btn" title="Xóa node">&times;</button>
        </div>
      `;

      // Delete Node
      header.querySelector('.wf-node-del-btn').addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteNode(node.id);
      });

      // Drag Node (Entire node card is smoothly draggable)
      let startClientX = 0, startClientY = 0;
      let startNodeX = 0, startNodeY = 0;
      let isDraggingNode = false;

      nodeEl.addEventListener('mousedown', (e) => {
        if (e.button !== 0) return;

        // Ignore interactive targets (port dots, add button, delete button, inputs, selects)
        if (e.target.closest('.wf-port-dot') ||
            e.target.closest('.wf-port-add-btn') ||
            e.target.closest('.wf-node-del-btn') ||
            e.target.closest('button') ||
            e.target.closest('input') ||
            e.target.closest('textarea') ||
            e.target.closest('select') ||
            e.target.closest('video') ||
            e.target.closest('a')) {
          return;
        }

        e.stopPropagation();
        e.preventDefault(); // Prevent browser text selection ghost drag

        this.selectNode(node.id);

        isDraggingNode = true;
        startClientX = e.clientX;
        startClientY = e.clientY;
        startNodeX = typeof node.x === 'number' ? node.x : (parseFloat(nodeEl.style.left) || 100);
        startNodeY = typeof node.y === 'number' ? node.y : (parseFloat(nodeEl.style.top) || 100);

        nodeEl.classList.add('is-dragging');
        document.body.style.userSelect = 'none';

        const onMouseMove = (ev) => {
          if (!isDraggingNode) return;
          const dx = (ev.clientX - startClientX) / this.scale;
          const dy = (ev.clientY - startClientY) / this.scale;

          node.x = Math.round(startNodeX + dx);
          node.y = Math.round(startNodeY + dy);

          nodeEl.style.left = `${node.x}px`;
          nodeEl.style.top = `${node.y}px`;

          this.renderWires();
        };

        const onMouseUp = () => {
          if (isDraggingNode) {
            isDraggingNode = false;
            nodeEl.classList.remove('is-dragging');
            document.body.style.userSelect = '';
          }
          window.removeEventListener('mousemove', onMouseMove);
          window.removeEventListener('mouseup', onMouseUp);
        };

        window.addEventListener('mousemove', onMouseMove);
        window.addEventListener('mouseup', onMouseUp);
      });

      nodeEl.appendChild(header);

      // 2. Ports Row (Inputs on Left, Outputs on Right with + Handle)
      const portsRow = document.createElement('div');
      portsRow.className = 'wf-ports-row';

      const inputsCol = document.createElement('div');
      inputsCol.className = 'wf-ports-inputs';
      const outputsCol = document.createElement('div');
      outputsCol.className = 'wf-ports-outputs';

      const inputs = node.inputs || def.inputs || [];
      inputs.forEach(port => {
        inputsCol.appendChild(this.createPortElement(node.id, port, false));
      });

      const outputs = node.outputs || def.outputs || [];
      outputs.forEach(port => {
        outputsCol.appendChild(this.createPortElement(node.id, port, true));
      });

      portsRow.appendChild(inputsCol);
      portsRow.appendChild(outputsCol);
      nodeEl.appendChild(portsRow);

      // 3. Node Footer & Status Indicator
      const footer = document.createElement('div');
      footer.className = 'wf-node-footer';
      footer.id = `node-footer-${node.id}`;
      footer.innerHTML = this.renderNodeStatus(node.state || { status: 'idle' });
      nodeEl.appendChild(footer);

      // Click anywhere on node to select
      nodeEl.addEventListener('click', (e) => {
        e.stopPropagation();
        this.selectNode(node.id);
      });

      this.nodesLayer.appendChild(nodeEl);
    }

    createPortElement(nodeId, port, isOutput) {
      const portEl = document.createElement('div');
      portEl.className = `wf-port ${isOutput ? 'wf-port-output' : 'wf-port-input'} port-type-${port.type || 'any'}`;
      portEl.dataset.nodeId = nodeId;
      portEl.dataset.portId = port.id;
      portEl.dataset.type = port.type || 'any';
      portEl.dataset.isOutput = isOutput ? '1' : '0';

      const dot = document.createElement('span');
      dot.className = 'wf-port-dot';
      dot.title = `${port.name} (${port.type})`;

      const label = document.createElement('span');
      label.className = 'wf-port-label';
      label.innerText = port.name;

      if (isOutput) {
        portEl.appendChild(label);
        portEl.appendChild(dot);

        // Add (+) quick connect handle button (n8n signature UX)
        const addBtn = document.createElement('span');
        addBtn.className = 'wf-port-add-btn';
        addBtn.innerText = '+';
        addBtn.title = 'Bấm để thêm bước tiếp theo và tự động nối dây';
        addBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          this.pendingConnect = {
            fromNode: nodeId,
            fromPort: port.id,
            portType: port.type
          };
          this.openAddNodeModal();
        });
        portEl.appendChild(addBtn);
      } else {
        portEl.appendChild(dot);
        portEl.appendChild(label);
      }

      // Drag to create wire from output dot
      dot.addEventListener('mousedown', (e) => {
        e.stopPropagation();
        if (!isOutput) return;

        const rect = dot.getBoundingClientRect();
        const containerRect = this.container.getBoundingClientRect();
        const startX = (rect.left + rect.width / 2 - containerRect.left - this.panX) / this.scale;
        const startY = (rect.top + rect.height / 2 - containerRect.top - this.panY) / this.scale;

        this.activeConnecting = {
          nodeId,
          portId: port.id,
          portType: port.type,
          startX,
          startY
        };

        this.createTempWire(startX, startY);
      });

      // Drop wire connection on input dot
      dot.addEventListener('mouseup', (e) => {
        e.stopPropagation();
        if (this.activeConnecting && !isOutput) {
          const fromNode = this.activeConnecting.nodeId;
          const fromPort = this.activeConnecting.portId;
          const toNode = nodeId;
          const toPort = port.id;

          if (fromNode !== toNode) {
            this.addEdge(fromNode, fromPort, toNode, toPort);
          }
          this.cancelConnecting();
        }
      });

      return portEl;
    }

    renderWires() {
      this.svgLayer.innerHTML = '';
      if (!this.graph.edges) return;

      this.graph.edges.forEach(edge => {
        const fromDot = this.findPortDot(edge.from_node, edge.from_port, true);
        const toDot = this.findPortDot(edge.to_node, edge.to_port, false);
        if (!fromDot || !toDot) return;

        const p1 = this.getPortCoordinates(fromDot);
        const p2 = this.getPortCoordinates(toDot);
        const d = this.calculateBezier(p1.x, p1.y, p2.x, p2.y);

        const fromNode = this.graph.nodes.find(n => n.id === edge.from_node);
        const toNode = this.graph.nodes.find(n => n.id === edge.to_node);
        const portType = fromDot.parentElement ? fromDot.parentElement.dataset.type : 'any';

        const isRunning = (fromNode?.state?.status === 'running') || (toNode?.state?.status === 'running');

        const group = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        group.setAttribute('class', `flow-wire-group ${isRunning ? 'active-flowing' : ''}`);
        group.dataset.edgeId = edge.id;

        // 1. Shadow / Glow outline path (ComfyUI & n8n depth)
        const shadowPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        shadowPath.setAttribute('d', d);
        shadowPath.setAttribute('class', 'flow-wire-shadow');
        group.appendChild(shadowPath);

        // 2. Core colored wire path
        const wirePath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        wirePath.setAttribute('d', d);
        wirePath.setAttribute('class', `flow-wire port-type-${portType} ${isRunning ? 'wire-active' : ''}`);
        group.appendChild(wirePath);

        // 3. Flowing stream pulse path if connected nodes are running
        if (isRunning) {
          const pulsePath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
          pulsePath.setAttribute('d', d);
          pulsePath.setAttribute('class', 'flow-wire-pulse');
          group.appendChild(pulsePath);
        }

        // 4. Hit area for easy clicking / hover
        const hitPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        hitPath.setAttribute('d', d);
        hitPath.setAttribute('class', 'flow-wire-hit');
        hitPath.addEventListener('click', (e) => {
          e.stopPropagation();
          if (confirm('Bạn có muốn xóa kết nối dây này?')) {
            this.deleteEdge(edge.id);
          }
        });
        group.appendChild(hitPath);

        this.svgLayer.appendChild(group);
      });
    }

    createTempWire(x, y) {
      this.tempWire = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      this.tempWire.setAttribute('class', 'flow-wire temp-wire');
      this.svgLayer.appendChild(this.tempWire);
    }

    updateTempWire(clientX, clientY) {
      if (!this.tempWire || !this.activeConnecting) return;
      const containerRect = this.container.getBoundingClientRect();
      const curX = (clientX - containerRect.left - this.panX) / this.scale;
      const curY = (clientY - containerRect.top - this.panY) / this.scale;

      const d = this.calculateBezier(this.activeConnecting.startX, this.activeConnecting.startY, curX, curY);
      this.tempWire.setAttribute('d', d);
    }

    cancelConnecting() {
      if (this.tempWire) {
        this.tempWire.remove();
        this.tempWire = null;
      }
      this.activeConnecting = null;
    }

    calculateBezier(x1, y1, x2, y2) {
      const dx = Math.max(50, Math.abs(x2 - x1) * 0.55);
      return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
    }

    findPortDot(nodeId, portId, isOutput) {
      const selector = `.wf-port[data-node-id="${nodeId}"][data-port-id="${portId}"][data-is-output="${isOutput ? '1' : '0'}"] .wf-port-dot`;
      return this.nodesLayer.querySelector(selector);
    }

    getPortCoordinates(dotEl) {
      const rect = dotEl.getBoundingClientRect();
      const containerRect = this.container.getBoundingClientRect();
      return {
        x: (rect.left + rect.width / 2 - containerRect.left - this.panX) / this.scale,
        y: (rect.top + rect.height / 2 - containerRect.top - this.panY) / this.scale
      };
    }

    addEdge(fromNode, fromPort, toNode, toPort) {
      // Remove any existing edge to the same input port
      this.graph.edges = this.graph.edges.filter(e => !(e.to_node === toNode && e.to_port === toPort));

      const newEdge = {
        id: `e_${Date.now()}_${Math.floor(Math.random() * 1000)}`,
        from_node: fromNode,
        from_port: fromPort,
        to_node: toNode,
        to_port: toPort
      };
      this.graph.edges.push(newEdge);
      this.renderWires();
    }

    deleteEdge(edgeId) {
      this.graph.edges = this.graph.edges.filter(e => e.id !== edgeId);
      this.renderWires();
    }

    deleteNode(nodeId) {
      this.graph.nodes = this.graph.nodes.filter(n => n.id !== nodeId);
      this.graph.edges = this.graph.edges.filter(e => e.from_node !== nodeId && e.to_node !== nodeId);
      if (this.selectedNodeId === nodeId) {
        this.deselectAll();
      }
      this.render();
    }

    addNode(nodeType, x, y) {
      const def = this.definitions[nodeType];
      if (!def) return;

      const id = `node_${Date.now()}`;
      const newNode = {
        id: id,
        type: nodeType,
        title: def.title,
        category: def.category,
        x: Math.round(x || 200),
        y: Math.round(y || 200),
        inputs: def.inputs ? JSON.parse(JSON.stringify(def.inputs)) : [],
        outputs: def.outputs ? JSON.parse(JSON.stringify(def.outputs)) : [],
        params: def.default_params ? JSON.parse(JSON.stringify(def.default_params)) : {},
        state: { status: 'idle', progress: 0 }
      };

      this.graph.nodes.push(newNode);

      // Auto-connect if pending from '+' handle
      if (this.pendingConnect) {
        const { fromNode, fromPort } = this.pendingConnect;
        const matchingInput = (newNode.inputs || []).find(inp => inp.type === this.pendingConnect.portType) || (newNode.inputs || [])[0];
        if (matchingInput) {
          this.addEdge(fromNode, fromPort, id, matchingInput.id);
        }
        this.pendingConnect = null;
      }

      this.render();
      this.selectNode(id);
    }

    selectNode(nodeId) {
      this.selectedNodeId = nodeId;
      this.nodesLayer.querySelectorAll('.wf-node').forEach(el => {
        el.classList.toggle('selected', el.id === `node-${nodeId}`);
      });
      this.openInspector();
      this.updateInspector();
    }

    deselectAll() {
      this.selectedNodeId = null;
      this.nodesLayer.querySelectorAll('.wf-node').forEach(el => el.classList.remove('selected'));
      this.updateInspector();
    }

    openInspector() {
      if (this.inspector) {
        this.inspector.classList.remove('collapsed');
      }
    }

    closeInspector() {
      if (this.inspector) {
        this.inspector.classList.add('collapsed');
      }
    }

    renderNodeStatus(state) {
      if (!state || state.status === 'idle') {
        return `<span class="wf-status-badge wf-status-idle">Chờ thực thi</span>`;
      }
      if (state.status === 'running') {
        return `<span class="wf-status-badge wf-status-running"><span class="wf-spinner"></span> ⚡ Đang thực thi (${state.progress || 10}%)...</span>`;
      }
      if (state.status === 'completed') {
        let preview = '';
        if (state.output_preview) {
          const fn = this.getFilename(state.output_preview);
          if (fn.endsWith('.mp4') || fn.endsWith('.webm')) {
            preview = `<div class="wf-node-preview-thumb"><video src="/media/${fn}" autoplay muted loop playsinline></video></div>`;
          } else {
            preview = `<div class="wf-node-preview-thumb"><img src="/media/${fn}" alt="Preview"></div>`;
          }
        }
        return `<div class="wf-status-badge wf-status-completed">✓ Hoàn tất 100%</div>${preview}`;
      }
      if (state.status === 'failed') {
        return `<span class="wf-status-badge wf-status-failed" title="${state.error || ''}">⚠️ Lỗi: ${state.error ? state.error.slice(0, 26) : 'Thất bại'}</span>`;
      }
      return '';
    }

    getFilename(path) {
      if (!path) return '';
      return path.split('/').pop();
    }

    getCategoryIcon(cat) {
      switch (cat) {
        case 'input': return '📥';
        case 'process': return '⚙️';
        case 'ai': return '🧠';
        case 'output': return '📤';
        default: return '📦';
      }
    }

    bindToolbar() {
      // Preset switcher
      const presetSelect = document.getElementById('wf-preset-select');
      if (presetSelect) {
        presetSelect.addEventListener('change', (e) => {
          window.location.href = `/workflow?preset=${encodeURIComponent(e.target.value)}`;
        });
      }

      // Add Node button
      const btnAdd = document.getElementById('btn-add-node');
      if (btnAdd) {
        btnAdd.addEventListener('click', () => {
          this.pendingConnect = null;
          this.openAddNodeModal();
        });
      }

      // Save Graph button
      const btnSave = document.getElementById('btn-save-graph');
      if (btnSave) {
        btnSave.addEventListener('click', async () => {
          try {
            const resp = await fetch('/api/flow/graphs', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(this.graph)
            });
            const data = await resp.json();
            if (data.ok) {
              alert('Đã lưu đồ thị workflow thành công!');
            } else {
              alert('Lỗi lưu đồ thị: ' + (data.error || ''));
            }
          } catch (err) {
            alert('Lỗi kết nối máy chủ: ' + err.message);
          }
        });
      }

      // Run Flow button
      const btnRun = document.getElementById('btn-run-flow');
      if (btnRun) {
        btnRun.addEventListener('click', () => {
          this.runFlow();
        });
      }

      // Cancel Flow button
      const btnCancel = document.getElementById('btn-cancel-flow');
      if (btnCancel) {
        btnCancel.addEventListener('click', () => {
          this.cancelFlow();
        });
      }

      // Zoom buttons
      const btnZoomIn = document.getElementById('btn-zoom-in');
      if (btnZoomIn) {
        btnZoomIn.addEventListener('click', () => {
          this.scale = Math.min(2.5, this.scale * 1.15);
          this.applyTransform();
          this.updateZoomDisplay();
        });
      }

      const btnZoomOut = document.getElementById('btn-zoom-out');
      if (btnZoomOut) {
        btnZoomOut.addEventListener('click', () => {
          this.scale = Math.max(0.35, this.scale / 1.15);
          this.applyTransform();
          this.updateZoomDisplay();
        });
      }

      const btnZoomReset = document.getElementById('btn-zoom-reset');
      if (btnZoomReset) {
        btnZoomReset.addEventListener('click', () => {
          this.scale = 1.0;
          this.panX = 40;
          this.panY = 40;
          this.applyTransform();
          this.updateZoomDisplay();
        });
      }

      // Sidebar toggle from workflow toolbar
      const wfSidebarToggle = document.getElementById('wf-sidebar-toggle');
      if (wfSidebarToggle) {
        wfSidebarToggle.addEventListener('click', () => {
          const layout = document.querySelector('.layout');
          if (!layout) return;
          const isCollapsed = layout.classList.toggle('sidebar-collapsed');
          try {
            localStorage.setItem('aicos-sidebar-collapsed', isCollapsed ? '1' : '0');
          } catch (e) {}
        });
      }

      // Theme toggle from workflow toolbar
      const wfThemeToggle = document.getElementById('wf-theme-toggle');
      if (wfThemeToggle) {
        wfThemeToggle.addEventListener('click', () => {
          const doc = document.documentElement;
          const cur = doc.dataset.theme === 'dark' ? 'light' : 'dark';
          doc.dataset.theme = cur;
          try {
            localStorage.setItem('aicos-theme', cur);
          } catch (e) {}
        });
      }
    }

    async runFlow() {
      const runBtn = document.getElementById('btn-run-flow');
      const cancelBtn = document.getElementById('btn-cancel-flow');
      const statusPill = this.statusPill;

      if (runBtn) runBtn.disabled = true;
      if (cancelBtn) cancelBtn.style.display = 'inline-flex';
      if (statusPill) {
        statusPill.className = 'wf-status-pill status-running';
        statusPill.innerText = '⚡ Đang chạy workflow...';
      }

      // Reset visual node states to queued
      this.graph.nodes.forEach(n => {
        n.state = { status: 'queued', progress: 0 };
        const footer = document.getElementById(`node-footer-${n.id}`);
        if (footer) footer.innerHTML = this.renderNodeStatus(n.state);
      });

      // Switch inspector to logs tab
      const logTabBtn = document.querySelector('.wf-tab[data-tab="run-logs"]');
      if (logTabBtn) logTabBtn.click();
      this.openInspector();

      this.clearLogs();
      this.appendLog('[Khởi chạy] Đang kiểm tra cấu hình đồ thị Workflow...');

      try {
        const resp = await fetch('/api/flow/execute', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ graph: this.graph })
        });
        const data = await resp.json();
        if (!data.ok) {
          throw new Error(data.error || 'Lỗi không xác định khi khởi chạy');
        }

        this.currentRunId = data.run_id;
        this.appendLog(`[DAG Engine] Đã kích hoạt phiên chạy ID: ${this.currentRunId}`);
        this.startPollingRun(this.currentRunId);
      } catch (err) {
        if (runBtn) runBtn.disabled = false;
        if (cancelBtn) cancelBtn.style.display = 'none';
        if (statusPill) {
          statusPill.className = 'wf-status-pill status-error';
          statusPill.innerText = '⚠️ Lỗi khởi chạy';
        }
        this.appendLog(`[LỖI] ${err.message}`);
        alert('Lỗi chạy workflow: ' + err.message);
      }
    }

    startPollingRun(runId) {
      if (this.pollInterval) clearInterval(this.pollInterval);

      this.pollInterval = setInterval(async () => {
        try {
          const resp = await fetch(`/api/flow/runs/${runId}`);
          const res = await resp.json();
          if (!res.ok || !res.run) return;

          const run = res.run;
          const progressText = document.getElementById('run-progress-text');
          if (progressText) {
            progressText.innerText = `${run.progress || 0}%`;
          }

          // Update terminal log
          if (run.log) {
            const term = document.getElementById('flow-logs-terminal');
            if (term) {
              term.innerText = run.log;
              term.scrollTop = term.scrollHeight;
            }
          }

          // Update node states
          if (run.node_states) {
            Object.keys(run.node_states).forEach(nodeId => {
              const nodeState = run.node_states[nodeId];
              const node = this.graph.nodes.find(n => n.id === nodeId);
              if (node) {
                node.state = nodeState;
                const footer = document.getElementById(`node-footer-${nodeId}`);
                if (footer) footer.innerHTML = this.renderNodeStatus(nodeState);

                const nodeEl = document.getElementById(`node-${nodeId}`);
                if (nodeEl) {
                  nodeEl.classList.toggle('node-running', nodeState.status === 'running');
                  nodeEl.classList.toggle('node-active-executing', nodeState.status === 'running');
                  nodeEl.classList.toggle('node-completed', nodeState.status === 'completed');
                  nodeEl.classList.toggle('node-failed', nodeState.status === 'failed');
                }
              }
            });
            this.renderWires(); // Re-render wires so active running wires light up
          }

          // Terminal completion
          if (run.status === 'completed' || run.status === 'failed' || run.status === 'cancelled') {
            clearInterval(this.pollInterval);
            this.pollInterval = null;

            const runBtn = document.getElementById('btn-run-flow');
            const cancelBtn = document.getElementById('btn-cancel-flow');
            if (runBtn) runBtn.disabled = false;
            if (cancelBtn) cancelBtn.style.display = 'none';

            const statusPill = this.statusPill;
            if (run.status === 'completed') {
              if (statusPill) {
                statusPill.className = 'wf-status-pill status-ready';
                statusPill.innerText = '✓ Hoàn tất 100%';
              }
              this.appendLog(`🎉 [Hoàn tất thành công] Video thành phẩm: ${run.final_output || 'Hoàn tất'}`);
              if (run.final_output) {
                this.showFinalModal(run.final_output);
              }
            } else if (run.status === 'failed') {
              if (statusPill) {
                statusPill.className = 'wf-status-pill status-error';
                statusPill.innerText = '⚠️ Thất bại';
              }
            } else {
              if (statusPill) {
                statusPill.className = 'wf-status-pill status-ready';
                statusPill.innerText = 'Đã dừng';
              }
            }
          }
        } catch (e) {
          console.error('Workflow poll error', e);
        }
      }, 700);
    }

    async cancelFlow() {
      if (!this.currentRunId) return;
      try {
        await fetch(`/api/flow/runs/${this.currentRunId}/cancel`, { method: 'POST' });
        this.appendLog('[Hủy bỏ] Đã gửi tín hiệu dừng tới hệ thống');
      } catch (e) {
        console.error(e);
      }
    }

    showFinalModal(outputUrl) {
      const filename = this.getFilename(outputUrl);
      const isVideo = filename.endsWith('.mp4') || filename.endsWith('.webm');

      const modal = document.createElement('div');
      modal.className = 'wf-modal-overlay';
      modal.innerHTML = `
        <div class="wf-modal-window" style="max-width:560px">
          <div class="wf-modal-top">
            <div class="wf-modal-title">
              <span class="wf-modal-title-icon">🎉</span>
              <h3>Video Thành Phẩm Workflow</h3>
            </div>
            <button type="button" class="wf-modal-close">&times;</button>
          </div>
          <div style="padding:20px;text-align:center">
            ${isVideo ? `<video src="/media/${filename}" controls autoplay style="width:100%;max-height:480px;border-radius:10px;background:#000;box-shadow:0 8px 24px rgba(0,0,0,0.3)"></video>` : `<img src="/media/${filename}" style="max-width:100%;border-radius:10px">`}
            <div style="margin-top:18px;display:flex;gap:12px;justify-content:center">
              <a href="/media/${filename}" download class="wf-btn wf-btn-primary">⬇ Tải video về máy</a>
              <a href="/studio/jobs" class="wf-btn wf-btn-secondary">Xem danh sách Video ↗</a>
            </div>
          </div>
        </div>
      `;
      modal.querySelector('.wf-modal-close').addEventListener('click', () => modal.remove());
      modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
      });
      document.body.appendChild(modal);
    }

    appendLog(msg) {
      const term = document.getElementById('flow-logs-terminal');
      if (!term) return;
      const line = document.createElement('div');
      line.className = 'wf-log-row';
      line.innerText = msg;
      term.appendChild(line);
      term.scrollTop = term.scrollHeight;
    }

    clearLogs() {
      const term = document.getElementById('flow-logs-terminal');
      if (term) term.innerHTML = '';
    }

    bindInspector() {
      // Close button
      const closeBtn = document.getElementById('btn-close-inspector');
      if (closeBtn) {
        closeBtn.addEventListener('click', () => {
          this.closeInspector();
        });
      }

      // Tab switching
      document.querySelectorAll('.wf-tab').forEach(tab => {
        tab.addEventListener('click', () => {
          document.querySelectorAll('.wf-tab').forEach(t => t.classList.remove('active'));
          document.querySelectorAll('.wf-tab-pane').forEach(c => c.style.display = 'none');
          tab.classList.add('active');
          const target = document.getElementById(`tab-${tab.dataset.tab}`);
          if (target) target.style.display = 'block';
        });
      });

      const clearLogBtn = document.getElementById('btn-clear-logs');
      if (clearLogBtn) {
        clearLogBtn.addEventListener('click', () => this.clearLogs());
      }
    }

    updateInspector() {
      const emptyEl = document.getElementById('inspector-empty');
      const formEl = document.getElementById('inspector-form');
      if (!emptyEl || !formEl) return;

      if (!this.selectedNodeId) {
        emptyEl.style.display = 'block';
        formEl.style.display = 'none';
        return;
      }

      const node = this.graph.nodes.find(n => n.id === this.selectedNodeId);
      if (!node) {
        emptyEl.style.display = 'block';
        formEl.style.display = 'none';
        return;
      }

      emptyEl.style.display = 'none';
      formEl.style.display = 'block';

      const def = this.definitions[node.type] || {};
      const cat = node.category || def.category || 'process';

      const iconEl = document.getElementById('inspector-node-icon');
      if (iconEl) iconEl.innerText = this.getCategoryIcon(cat);

      const titleEl = document.getElementById('inspector-node-title');
      if (titleEl) titleEl.innerText = node.title || def.title || node.type;

      const catEl = document.getElementById('inspector-node-category');
      if (catEl) catEl.innerText = cat.toUpperCase();

      const descEl = document.getElementById('inspector-node-desc');
      if (descEl) descEl.innerText = def.description || '';

      const fieldsContainer = document.getElementById('inspector-fields');
      if (fieldsContainer) {
        fieldsContainer.innerHTML = '';

        if (!node.params) node.params = {};
        const params = Object.assign({}, def.default_params, node.params);

        Object.keys(params).forEach(key => {
          const val = params[key];
          const group = document.createElement('div');
          group.className = 'form-group';

          const label = document.createElement('label');
          label.innerText = this.formatParamLabel(key);
          group.appendChild(label);

          if (typeof val === 'boolean') {
            const chk = document.createElement('input');
            chk.type = 'checkbox';
            chk.checked = val;
            chk.addEventListener('change', () => {
              node.params[key] = chk.checked;
            });
            group.appendChild(chk);
          } else if (typeof val === 'number') {
            const numInput = document.createElement('input');
            numInput.type = 'number';
            numInput.className = 'form-control';
            numInput.value = val;
            numInput.addEventListener('input', () => {
              node.params[key] = parseFloat(numInput.value) || 0;
            });
            group.appendChild(numInput);
          } else if (key.includes('prompt') || key.includes('template') || (typeof val === 'string' && val.length > 50)) {
            const textarea = document.createElement('textarea');
            textarea.className = 'form-control';
            textarea.rows = 4;
            textarea.value = val;
            textarea.addEventListener('input', () => {
              node.params[key] = textarea.value;
            });
            group.appendChild(textarea);
          } else {
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'form-control';
            input.value = val;
            input.addEventListener('input', () => {
              node.params[key] = input.value;
            });
            group.appendChild(input);
          }

          fieldsContainer.appendChild(group);
        });
      }

      // Preview box
      const previewBox = document.getElementById('inspector-preview-box');
      const previewStatus = document.getElementById('inspector-preview-status');
      if (previewBox) {
        if (node.state && node.state.output_preview) {
          const fn = this.getFilename(node.state.output_preview);
          if (previewStatus) previewStatus.innerText = 'Đã có kết quả';
          if (fn.endsWith('.mp4') || fn.endsWith('.webm')) {
            previewBox.innerHTML = `<video src="/media/${fn}" controls autoplay muted loop style="max-width:100%;border-radius:6px"></video>`;
          } else {
            previewBox.innerHTML = `<img src="/media/${fn}" alt="Preview" style="max-width:100%;border-radius:6px">`;
          }
        } else {
          if (previewStatus) previewStatus.innerText = 'Chưa có kết quả';
          previewBox.innerHTML = '<span class="text-muted">Chưa có dữ liệu xuất</span>';
        }
      }
    }

    formatParamLabel(key) {
      const labels = {
        template: 'Mẫu câu lệnh (Template)',
        prompt: 'Câu lệnh Prompt',
        model_name: 'Tên nhân vật mẫu',
        product_title: 'Tên sản phẩm',
        aspect: 'Tỉ lệ khung hình (Aspect)',
        placement: 'Vị trí ghép sản phẩm',
        scale: 'Tỉ lệ phóng to',
        resolution: 'Độ phân giải',
        expression: 'Biểu cảm & Điệu nhảy',
        provider_name: 'Tên Web AI',
        target_url: 'Đường dẫn đăng nhập AI',
        auto_publish_draft: 'Tự động xuất TikTok nháp'
      };
      return labels[key] || key.replace(/_/g, ' ');
    }

    openAddNodeModal() {
      const modal = document.getElementById('add-node-modal');
      if (modal) {
        modal.style.display = 'flex';
        const searchInput = document.getElementById('node-search-input');
        if (searchInput) {
          searchInput.value = '';
          searchInput.focus();
        }
        this.renderModalGrid('', 'all');
      }
    }

    closeAddNodeModal() {
      const modal = document.getElementById('add-node-modal');
      if (modal) {
        modal.style.display = 'none';
      }
      this.pendingConnect = null;
    }

    bindModal() {
      const modal = document.getElementById('add-node-modal');
      const closeBtn = document.getElementById('btn-close-node-modal');
      const searchInput = document.getElementById('node-search-input');

      if (closeBtn) {
        closeBtn.addEventListener('click', () => this.closeAddNodeModal());
      }

      if (modal) {
        modal.addEventListener('click', (e) => {
          if (e.target === modal) this.closeAddNodeModal();
        });
      }

      if (searchInput) {
        searchInput.addEventListener('input', (e) => {
          const activeChip = document.querySelector('.wf-tag-filter.active');
          const activeCat = activeChip ? activeChip.dataset.cat : 'all';
          this.renderModalGrid(e.target.value, activeCat);
        });
      }

      document.querySelectorAll('.wf-tag-filter').forEach(chip => {
        chip.addEventListener('click', () => {
          document.querySelectorAll('.wf-tag-filter').forEach(c => c.classList.remove('active'));
          chip.classList.add('active');
          const query = searchInput ? searchInput.value : '';
          this.renderModalGrid(query, chip.dataset.cat);
        });
      });
    }

    renderModalGrid(filterText = '', filterCat = 'all') {
      const grid = document.getElementById('modal-node-grid');
      if (!grid) return;
      grid.innerHTML = '';

      Object.values(this.definitions).forEach(def => {
        if (filterCat !== 'all' && def.category !== filterCat) return;
        if (filterText) {
          const query = filterText.toLowerCase();
          const matchTitle = def.title.toLowerCase().includes(query);
          const matchDesc = (def.description || '').toLowerCase().includes(query);
          const matchType = def.type.toLowerCase().includes(query);
          if (!matchTitle && !matchDesc && !matchType) return;
        }

        const cat = def.category || 'process';
        const card = document.createElement('div');
        card.className = `wf-library-card cat-${cat}`;
        card.innerHTML = `
          <div class="wf-card-top">
            <div class="wf-card-icon-box cat-${cat}">
              <span>${this.getCategoryIcon(cat)}</span>
            </div>
            <div>
              <h4 class="wf-card-title">${def.title}</h4>
            </div>
          </div>
          <p class="wf-card-desc">${def.description || ''}</p>
          <div class="wf-card-bottom">
            <span class="wf-card-cat-badge">${cat.toUpperCase()}</span>
            <span class="wf-card-add-text">+ Thêm vào Canvas</span>
          </div>
        `;

        card.addEventListener('click', () => {
          let posX = 200, posY = 200;
          if (this.pendingConnect) {
            // Position near the source node
            const parentNode = this.graph.nodes.find(n => n.id === this.pendingConnect.fromNode);
            if (parentNode) {
              posX = (parentNode.x || 100) + 320;
              posY = (parentNode.y || 100);
            }
          } else {
            // Place near screen center
            const containerRect = this.container.getBoundingClientRect();
            posX = (-this.panX + containerRect.width / 2 - 135) / this.scale;
            posY = (-this.panY + containerRect.height / 2 - 60) / this.scale;
          }

          this.addNode(def.type, posX, posY);
          this.closeAddNodeModal();
        });

        grid.appendChild(card);
      });
    }
  }

  function initCanvas() {
    if (!window.flowCanvas) {
      window.flowCanvas = new FlowCanvas();
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initCanvas);
  } else {
    initCanvas();
  }
})();