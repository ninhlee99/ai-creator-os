// FlowCanvas - Interactive Node Graph Workflow Canvas (ComfyUI / n8n style)
(function () {
  'use strict';

  class FlowCanvas {
    constructor() {
      this.container = document.getElementById('flow-canvas-container');
      this.viewport = document.getElementById('flow-canvas-viewport');
      this.svgLayer = document.getElementById('flow-wires-svg');
      this.nodesLayer = document.getElementById('flow-nodes-layer');

      this.graph = window.__INITIAL_GRAPH__ || { id: 'custom_flow', name: 'Luồng tùy chỉnh', nodes: [], edges: [] };
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
      this.activeConnecting = null; // { nodeId, portId, isOutput, x, y }
      this.tempWire = null;

      this.currentRunId = null;
      this.pollInterval = null;

      this.initEvents();
      this.render();
      this.bindToolbar();
      this.bindInspector();
      this.bindModal();
    }

    initEvents() {
      // Pan canvas
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

      // Zoom via mouse wheel
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

        // Zoom toward mouse position
        this.panX = mouseX - (mouseX - this.panX) * (this.scale / prevScale);
        this.panY = mouseY - (mouseY - this.panY) * (this.scale / prevScale);

        this.applyTransform();
        this.updateZoomDisplay();
      }, { passive: false });

      // Window resize
      window.addEventListener('resize', () => {
        this.renderWires();
      });

      // Delete node / wire on Delete key
      window.addEventListener('keydown', (e) => {
        if (e.key === 'Delete' || e.key === 'Backspace') {
          if (document.activeElement.tagName === 'INPUT' || document.activeElement.tagName === 'TEXTAREA') return;
          if (this.selectedNodeId) {
            this.deleteNode(this.selectedNodeId);
          }
        }
      });
    }

    applyTransform() {
      this.viewport.style.transform = `translate(${this.panX}px, ${this.panY}px) scale(${this.scale})`;
      this.viewport.style.transformOrigin = '0 0';
    }

    updateZoomDisplay() {
      const zEl = document.getElementById('zoom-level');
      if (zEl) zEl.innerText = `${Math.round(this.scale * 100)}%`;
    }

    render() {
      this.nodesLayer.innerHTML = '';
      this.svgLayer.innerHTML = '';

      if (!this.graph.nodes) this.graph.nodes = [];
      if (!this.graph.edges) this.graph.edges = [];

      this.graph.nodes.forEach(node => this.renderNode(node));
      this.renderWires();
      this.applyTransform();
    }

    renderNode(node) {
      const def = this.definitions[node.Type] || this.definitions[node.type] || {};
      const nodeEl = document.createElement('div');
      nodeEl.className = `flow-node node-cat-${node.category || def.category || 'process'}`;
      nodeEl.id = `node-${node.id}`;
      nodeEl.style.left = `${node.x || 100}px`;
      nodeEl.style.top = `${node.y || 100}px`;

      if (node.id === this.selectedNodeId) {
        nodeEl.classList.add('selected');
      }

      // Card Header
      const header = document.createElement('div');
      header.className = 'node-header';
      header.innerHTML = `
        <div class="node-title-group">
          <span class="node-icon">${this.getCategoryIcon(node.category || def.category)}</span>
          <span class="node-title">${node.title || def.title || node.type}</span>
        </div>
        <div class="node-header-actions">
          <button type="button" class="node-btn-del" title="Xóa node">&times;</button>
        </div>
      `;

      // Delete button
      header.querySelector('.node-btn-del').addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteNode(node.id);
      });

      // Drag node
      let startX = 0, startY = 0;
      header.addEventListener('mousedown', (e) => {
        if (e.button !== 0) return;
        e.stopPropagation();
        this.selectNode(node.id);

        startX = e.clientX;
        startY = e.clientY;

        const onMouseMove = (ev) => {
          const dx = (ev.clientX - startX) / this.scale;
          const dy = (ev.clientY - startY) / this.scale;
          node.x = (node.x || 0) + dx;
          node.y = (node.y || 0) + dy;
          nodeEl.style.left = `${node.x}px`;
          nodeEl.style.top = `${node.y}px`;
          startX = ev.clientX;
          startY = ev.clientY;
          this.renderWires();
        };

        const onMouseUp = () => {
          window.removeEventListener('mousemove', onMouseMove);
          window.removeEventListener('mouseup', onMouseUp);
        };

        window.addEventListener('mousemove', onMouseMove);
        window.addEventListener('mouseup', onMouseUp);
      });

      nodeEl.appendChild(header);

      // Node Body: Ports
      const body = document.createElement('div');
      body.className = 'node-body';

      const inputsCol = document.createElement('div');
      inputsCol.className = 'node-ports-col inputs';
      const outputsCol = document.createElement('div');
      outputsCol.className = 'node-ports-col outputs';

      const inputs = node.inputs || def.inputs || [];
      inputs.forEach(port => {
        inputsCol.appendChild(this.createPortElement(node.id, port, false));
      });

      const outputs = node.outputs || def.outputs || [];
      outputs.forEach(port => {
        outputsCol.appendChild(this.createPortElement(node.id, port, true));
      });

      body.appendChild(inputsCol);
      body.appendChild(outputsCol);
      nodeEl.appendChild(body);

      // Node Preview / Status Footer
      const footer = document.createElement('div');
      footer.className = 'node-footer';
      footer.id = `node-footer-${node.id}`;

      const state = node.state || { status: 'idle' };
      footer.innerHTML = this.renderNodeStatus(state);
      nodeEl.appendChild(footer);

      // Select node on click
      nodeEl.addEventListener('click', (e) => {
        e.stopPropagation();
        this.selectNode(node.id);
      });

      this.nodesLayer.appendChild(nodeEl);
    }

    createPortElement(nodeId, port, isOutput) {
      const portEl = document.createElement('div');
      portEl.className = `flow-port ${isOutput ? 'port-output' : 'port-input'} port-type-${port.type || 'any'}`;
      portEl.dataset.nodeId = nodeId;
      portEl.dataset.portId = port.id;
      portEl.dataset.type = port.type || 'any';
      portEl.dataset.isOutput = isOutput ? '1' : '0';

      const dot = document.createElement('span');
      dot.className = 'port-dot';
      dot.title = `${port.name} (${port.type})`;

      const label = document.createElement('span');
      label.className = 'port-label';
      label.innerText = port.name;

      if (isOutput) {
        portEl.appendChild(label);
        portEl.appendChild(dot);
      } else {
        portEl.appendChild(dot);
        portEl.appendChild(label);
      }

      // Drag to connect wire
      dot.addEventListener('mousedown', (e) => {
        e.stopPropagation();
        if (!isOutput) return; // Chỉ kéo từ output port sang input port

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

      // Drop connection on input port
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

        const path = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        const d = this.calculateBezier(p1.x, p1.y, p2.x, p2.y);
        path.setAttribute('d', d);
        path.setAttribute('class', 'flow-wire');
        path.dataset.edgeId = edge.id;

        // Click to delete wire
        path.addEventListener('click', (e) => {
          e.stopPropagation();
          if (confirm('Bạn có muốn xóa kết nối dây này?')) {
            this.deleteEdge(edge.id);
          }
        });

        this.svgLayer.appendChild(path);
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
      const selector = `.flow-port[data-node-id="${nodeId}"][data-port-id="${portId}"][data-is-output="${isOutput ? '1' : '0'}"] .port-dot`;
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
        x: x || 200,
        y: y || 200,
        inputs: def.inputs ? JSON.parse(JSON.stringify(def.inputs)) : [],
        outputs: def.outputs ? JSON.parse(JSON.stringify(def.outputs)) : [],
        params: def.default_params ? JSON.parse(JSON.stringify(def.default_params)) : {},
        state: { status: 'idle', progress: 0 }
      };

      this.graph.nodes.push(newNode);
      this.render();
      this.selectNode(id);
    }

    selectNode(nodeId) {
      this.selectedNodeId = nodeId;
      this.nodesLayer.querySelectorAll('.flow-node').forEach(el => {
        el.classList.toggle('selected', el.id === `node-${nodeId}`);
      });
      this.updateInspector();
    }

    deselectAll() {
      this.selectedNodeId = null;
      this.nodesLayer.querySelectorAll('.flow-node').forEach(el => el.classList.remove('selected'));
      this.updateInspector();
    }

    renderNodeStatus(state) {
      if (!state || state.status === 'idle') {
        return `<span class="status-badge idle">Chờ thực thi</span>`;
      }
      if (state.status === 'running') {
        return `<div class="status-running-box"><span class="spinner"></span> <span>Đang xử lý (${state.progress || 10}%)...</span></div>`;
      }
      if (state.status === 'completed') {
        let preview = '';
        if (state.output_preview) {
          if (state.output_preview.endsWith('.mp4') || state.output_preview.endsWith('.webm')) {
            preview = `<div class="node-media-preview"><video src="/media/${this.getFilename(state.output_preview)}" autoplay muted loop playsinline></video></div>`;
          } else {
            preview = `<div class="node-media-preview"><img src="/media/${this.getFilename(state.output_preview)}" alt="Preview"></div>`;
          }
        }
        return `<div class="status-badge completed"><span class="check-icon">✓</span> Hoàn tất 100%</div>${preview}`;
      }
      if (state.status === 'failed') {
        return `<div class="status-badge failed" title="${state.error || ''}">⚠️ Lỗi: ${state.error ? state.error.slice(0, 30) : 'Thất bại'}</div>`;
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
        case 'ai': return '✨';
        case 'output': return '📤';
        default: return '📦';
      }
    }

    bindToolbar() {
      // Preset switcher
      const presetSelect = document.getElementById('flow-preset-select');
      if (presetSelect) {
        presetSelect.addEventListener('change', (e) => {
          window.location.href = `/studio/flow?preset=${encodeURIComponent(e.target.value)}`;
        });
      }

      // Add Node button
      document.getElementById('btn-add-node').addEventListener('click', () => {
        document.getElementById('add-node-modal').style.display = 'flex';
      });

      // Save Graph button
      document.getElementById('btn-save-graph').addEventListener('click', async () => {
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

      // Run Flow button
      document.getElementById('btn-run-flow').addEventListener('click', () => {
        this.runFlow();
      });

      // Cancel Flow button
      document.getElementById('btn-cancel-flow').addEventListener('click', () => {
        this.cancelFlow();
      });

      // Zoom buttons
      document.getElementById('btn-zoom-in').addEventListener('click', () => {
        this.scale = Math.min(2.5, this.scale * 1.15);
        this.applyTransform();
        this.updateZoomDisplay();
      });
      document.getElementById('btn-zoom-out').addEventListener('click', () => {
        this.scale = Math.max(0.35, this.scale / 1.15);
        this.applyTransform();
        this.updateZoomDisplay();
      });
      document.getElementById('btn-zoom-reset').addEventListener('click', () => {
        this.scale = 1.0;
        this.panX = 40;
        this.panY = 40;
        this.applyTransform();
        this.updateZoomDisplay();
      });
    }

    async runFlow() {
      const runBtn = document.getElementById('btn-run-flow');
      const cancelBtn = document.getElementById('btn-cancel-flow');
      const statusEl = document.getElementById('flow-run-status');

      runBtn.disabled = true;
      cancelBtn.style.display = 'inline-flex';
      statusEl.className = 'flow-run-status status-running';
      statusEl.innerHTML = '<span class="status-dot pulsing"></span> <span class="status-text">Đang chạy...</span>';

      // Reset node visual states to queued
      this.graph.nodes.forEach(n => {
        n.state = { status: 'queued', progress: 0 };
        const footer = document.getElementById(`node-footer-${n.id}`);
        if (footer) footer.innerHTML = this.renderNodeStatus(n.state);
      });

      // Switch to log tab
      document.querySelector('.inspector-tab[data-tab="run-logs"]').click();
      this.clearLogs();
      this.appendLog('[Khởi chạy] Đang gửi cấu hình đồ thị tới máy chủ...');

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
        this.appendLog(`[Thực thi] Mã phiên chạy: ${this.currentRunId}`);
        this.startPollingRun(this.currentRunId);
      } catch (err) {
        runBtn.disabled = false;
        cancelBtn.style.display = 'none';
        statusEl.className = 'flow-run-status status-error';
        statusEl.innerHTML = '<span class="status-dot"></span> <span class="status-text">Lỗi khởi chạy</span>';
        this.appendLog(`[LỖI] ${err.message}`);
        alert('Lỗi chạy luồng: ' + err.message);
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
          document.getElementById('run-progress-text').innerText = `${run.progress || 0}%`;

          // Update terminal log
          if (run.log) {
            document.getElementById('flow-logs-terminal').innerText = run.log;
            const term = document.getElementById('flow-logs-terminal');
            term.scrollTop = term.scrollHeight;
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
                  nodeEl.classList.toggle('node-completed', nodeState.status === 'completed');
                  nodeEl.classList.toggle('node-failed', nodeState.status === 'failed');
                }
              }
            });
          }

          // Check terminal completion
          if (run.status === 'completed' || run.status === 'failed' || run.status === 'cancelled') {
            clearInterval(this.pollInterval);
            this.pollInterval = null;

            document.getElementById('btn-run-flow').disabled = false;
            document.getElementById('btn-cancel-flow').style.display = 'none';

            const statusEl = document.getElementById('flow-run-status');
            if (run.status === 'completed') {
              statusEl.className = 'flow-run-status status-completed';
              statusEl.innerHTML = '<span class="status-dot"></span> <span class="status-text">Hoàn tất 100%</span>';
              this.appendLog(`🎉 [Hoàn thành] Video cuối cùng: ${run.final_output || 'Thành công'}`);
              if (run.final_output) {
                this.showFinalModal(run.final_output);
              }
            } else if (run.status === 'failed') {
              statusEl.className = 'flow-run-status status-error';
              statusEl.innerHTML = '<span class="status-dot"></span> <span class="status-text">Thất bại</span>';
            } else {
              statusEl.className = 'flow-run-status status-idle';
              statusEl.innerHTML = '<span class="status-dot"></span> <span class="status-text">Đã hủy</span>';
            }
          }
        } catch (e) {
          console.error('Polling error', e);
        }
      }, 700);
    }

    async cancelFlow() {
      if (!this.currentRunId) return;
      try {
        await fetch(`/api/flow/runs/${this.currentRunId}/cancel`, { method: 'POST' });
        this.appendLog('[Hủy bỏ] Đã gửi tín hiệu hủy phiên thực thi');
      } catch (e) {
        console.error(e);
      }
    }

    showFinalModal(outputUrl) {
      const filename = this.getFilename(outputUrl);
      const isVideo = filename.endsWith('.mp4') || filename.endsWith('.webm');

      const modal = document.createElement('div');
      modal.className = 'flow-modal';
      modal.style.display = 'flex';
      modal.innerHTML = `
        <div class="flow-modal-dialog" style="max-width:540px">
          <div class="flow-modal-header">
            <h3>🎉 Video Thành Phẩm Studio ComfyUI</h3>
            <button type="button" class="flow-modal-close">&times;</button>
          </div>
          <div style="padding:16px;text-align:center">
            ${isVideo ? `<video src="/media/${filename}" controls autoplay style="width:100%;max-height:480px;border-radius:8px;background:#000"></video>` : `<img src="/media/${filename}" style="max-width:100%;border-radius:8px">`}
            <div style="margin-top:16px;display:flex;gap:10px;justify-content:center">
              <a href="/media/${filename}" download class="btn btn-primary">⬇ Tải video về máy</a>
              <a href="/studio/jobs" class="btn btn-secondary">Xem trong danh sách Video ↗</a>
            </div>
          </div>
        </div>
      `;
      modal.querySelector('.flow-modal-close').addEventListener('click', () => modal.remove());
      document.body.appendChild(modal);
    }

    appendLog(msg) {
      const term = document.getElementById('flow-logs-terminal');
      if (!term) return;
      const line = document.createElement('div');
      line.className = 'log-line';
      line.innerText = msg;
      term.appendChild(line);
      term.scrollTop = term.scrollHeight;
    }

    clearLogs() {
      const term = document.getElementById('flow-logs-terminal');
      if (term) term.innerHTML = '';
    }

    bindInspector() {
      document.querySelectorAll('.inspector-tab').forEach(tab => {
        tab.addEventListener('click', () => {
          document.querySelectorAll('.inspector-tab').forEach(t => t.classList.remove('active'));
          document.querySelectorAll('.inspector-content').forEach(c => c.style.display = 'none');
          tab.classList.add('active');
          const target = document.getElementById(`tab-${tab.dataset.tab}`);
          if (target) target.style.display = 'block';
        });
      });

      document.getElementById('btn-clear-logs').addEventListener('click', () => this.clearLogs());
    }

    updateInspector() {
      const emptyEl = document.getElementById('inspector-empty');
      const formEl = document.getElementById('inspector-form');
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
      document.getElementById('inspector-node-title').innerText = node.title || def.title || node.type;
      document.getElementById('inspector-node-category').innerText = (node.category || def.category || 'process').toUpperCase();
      document.getElementById('inspector-node-desc').innerText = def.description || '';

      const fieldsContainer = document.getElementById('inspector-fields');
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

      // Preview box
      const previewBox = document.getElementById('inspector-preview-box');
      if (node.state && node.state.output_preview) {
        const preview = node.state.output_preview;
        const filename = this.getFilename(preview);
        if (filename.endsWith('.mp4') || filename.endsWith('.webm')) {
          previewBox.innerHTML = `<video src="/media/${filename}" controls autoplay muted loop style="max-width:100%;border-radius:6px"></video>`;
        } else {
          previewBox.innerHTML = `<img src="/media/${filename}" alt="Preview" style="max-width:100%;border-radius:6px">`;
        }
      } else {
        previewBox.innerHTML = '<span class="muted">Chưa có kết quả chạy cho node này</span>';
      }
    }

    formatParamLabel(key) {
      const labels = {
        template: 'Mẫu câu lệnh (Template)',
        prompt: 'Câu lệnh Prompt',
        model_name: 'Tên nhân vật',
        product_title: 'Tên sản phẩm',
        aspect: 'Tỉ lệ khung hình (Aspect)',
        placement: 'Vị trí ghép sản phẩm',
        scale: 'Tỉ lệ phóng to sản phẩm',
        resolution: 'Độ phân giải',
        expression: 'Biểu cảm vũ đạo',
        provider_name: 'Tên nhà cung cấp Web AI',
        target_url: 'Đường dẫn đăng nhập AI',
        auto_publish_draft: 'Tự động tạo bản nháp TikTok'
      };
      return labels[key] || key.replace(/_/g, ' ');
    }

    bindModal() {
      const modal = document.getElementById('add-node-modal');
      const closeBtn = document.getElementById('btn-close-node-modal');
      const searchInput = document.getElementById('node-search-input');
      const grid = document.getElementById('modal-node-grid');

      closeBtn.addEventListener('click', () => {
        modal.style.display = 'none';
      });

      const renderGrid = (filterText = '', filterCat = 'all') => {
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

          const card = document.createElement('div');
          card.className = `modal-node-card cat-${def.category}`;
          card.innerHTML = `
            <div class="card-header">
              <span class="card-icon">${this.getCategoryIcon(def.category)}</span>
              <h4>${def.title}</h4>
            </div>
            <p>${def.description || ''}</p>
            <div class="card-footer">
              <span class="badge">${def.category.toUpperCase()}</span>
              <button type="button" class="btn btn-sm btn-primary">Thêm vào Canvas</button>
            </div>
          `;

          card.addEventListener('click', () => {
            // Thêm node vào giữa màn hình
            const containerRect = this.container.getBoundingClientRect();
            const centerX = (-this.panX + containerRect.width / 2 - 120) / this.scale;
            const centerY = (-this.panY + containerRect.height / 2 - 60) / this.scale;
            this.addNode(def.type, centerX, centerY);
            modal.style.display = 'none';
          });

          grid.appendChild(card);
        });
      };

      searchInput.addEventListener('input', (e) => {
        const activeCat = document.querySelector('.category-chip.active').dataset.cat;
        renderGrid(e.target.value, activeCat);
      });

      document.querySelectorAll('.category-chip').forEach(chip => {
        chip.addEventListener('click', () => {
          document.querySelectorAll('.category-chip').forEach(c => c.classList.remove('active'));
          chip.classList.add('active');
          renderGrid(searchInput.value, chip.dataset.cat);
        });
      });

      renderGrid();
    }
  }

  document.addEventListener('DOMContentLoaded', () => {
    window.flowCanvas = new FlowCanvas();
  });
})();
