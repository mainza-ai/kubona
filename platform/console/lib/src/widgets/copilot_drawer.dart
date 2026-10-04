import 'package:flutter/material.dart';
import '../api_client.dart';
import '../theme/observatory_theme.dart';

class CopilotDrawer extends StatefulWidget {
  final ConsoleApi api;
  final VoidCallback onExperimentTriggered;

  const CopilotDrawer({
    super.key,
    required this.api,
    required this.onExperimentTriggered,
  });

  @override
  State<CopilotDrawer> createState() => _CopilotDrawerState();
}

class _CopilotDrawerState extends State<CopilotDrawer> {
  final TextEditingController _controller = TextEditingController();
  final List<Map<String, dynamic>> _messages = [
    {
      'role': 'assistant',
      'content': 'Greetings, Principal Investigator. I am Kubona\'s Quantum Epistemic Copilot. Enter your molecular, Hamiltonian, or quantum research goal to synthesize an experiment proposal with verified classical anchors.',
      'proposal': null,
    }
  ];
  bool _loading = false;

  void _send() async {
    final text = _controller.text.trim();
    if (text.isEmpty || _loading) return;

    _controller.clear();
    setState(() {
      _messages.add({'role': 'user', 'content': text, 'proposal': null});
      _loading = true;
    });

    try {
      final res = await widget.api.chatMessages(text);
      if (mounted) {
        setState(() {
          _messages.add({
            'role': 'assistant',
            'content': res['content'] ?? '',
            'proposal': res['proposal_card'],
          });
          _loading = false;
        });
      }
    } catch (e) {
      if (mounted) {
        setState(() {
          _messages.add({
            'role': 'assistant',
            'content': 'Error communicating with research core: $e',
            'proposal': null,
          });
          _loading = false;
        });
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    return Drawer(
      backgroundColor: ObservatoryTheme.surface1,
      child: Column(
        children: [
          Container(
            padding: const EdgeInsets.only(top: 40, left: 16, right: 16, bottom: 16),
            decoration: const BoxDecoration(
              border: Border(bottom: BorderSide(color: ObservatoryTheme.borderSubtle)),
            ),
            child: Row(
              children: [
                const Icon(Icons.psychology, color: ObservatoryTheme.cyanCoherence),
                const SizedBox(width: 8),
                const Text(
                  'RESEARCH COPILOT',
                  style: TextStyle(
                    fontFamily: 'monospace',
                    fontWeight: FontWeight.bold,
                    letterSpacing: 1.2,
                    fontSize: 14,
                  ),
                ),
                const Spacer(),
                IconButton(
                  icon: const Icon(Icons.close, size: 18),
                  onPressed: () => Navigator.of(context).pop(),
                ),
              ],
            ),
          ),
          Expanded(
            child: ListView.builder(
              padding: const EdgeInsets.all(12),
              itemCount: _messages.length,
              itemBuilder: (context, idx) {
                final m = _messages[idx];
                final isUser = m['role'] == 'user';
                final proposal = m['proposal'] as Map<String, dynamic>?;

                return Column(
                  crossAxisAlignment:
                      isUser ? CrossAxisAlignment.end : CrossAxisAlignment.start,
                  children: [
                    Container(
                      margin: const EdgeInsets.symmetric(vertical: 6),
                      padding: const EdgeInsets.all(12),
                      decoration: BoxDecoration(
                        color: isUser
                            ? ObservatoryTheme.electricViolet.withOpacity(0.18)
                            : ObservatoryTheme.surface2,
                        borderRadius: BorderRadius.circular(8),
                        border: Border.all(
                          color: isUser
                              ? ObservatoryTheme.borderViolet
                              : ObservatoryTheme.borderSubtle,
                        ),
                      ),
                      child: Text(
                        m['content'] ?? '',
                        style: const TextStyle(fontSize: 13, height: 1.4),
                      ),
                    ),
                    if (proposal != null) _buildProposalCard(proposal),
                  ],
                );
              },
            ),
          ),
          if (_loading)
            const Padding(
              padding: EdgeInsets.symmetric(vertical: 8),
              child: Center(
                child: SizedBox(
                  height: 16,
                  width: 16,
                  child: CircularProgressIndicator(
                    strokeWidth: 2,
                    valueColor: AlwaysStoppedAnimation(ObservatoryTheme.cyanCoherence),
                  ),
                ),
              ),
            ),
          Container(
            padding: const EdgeInsets.all(12),
            decoration: const BoxDecoration(
              border: Border(top: BorderSide(color: ObservatoryTheme.borderSubtle)),
            ),
            child: Row(
              children: [
                Expanded(
                  child: TextField(
                    controller: _controller,
                    onSubmitted: (_) => _send(),
                    decoration: InputDecoration(
                      hintText: 'Pose hypothesis or query (e.g. H2 ground state)...',
                      hintStyle: TextStyle(fontSize: 12, color: Colors.white.withOpacity(0.4)),
                      filled: true,
                      fillColor: ObservatoryTheme.voidBg,
                      contentPadding: const EdgeInsets.symmetric(horizontal: 12, vertical: 8),
                      border: OutlineInputBorder(
                        borderRadius: BorderRadius.circular(6),
                        borderSide: const BorderSide(color: ObservatoryTheme.borderSubtle),
                      ),
                    ),
                    style: const TextStyle(fontSize: 13),
                  ),
                ),
                const SizedBox(width: 8),
                IconButton(
                  icon: const Icon(Icons.send, color: ObservatoryTheme.cyanCoherence),
                  onPressed: _send,
                ),
              ],
            ),
          ),
        ],
      ),
    );
  }

  Widget _buildProposalCard(Map<String, dynamic> p) {
    return Container(
      margin: const EdgeInsets.symmetric(vertical: 8),
      padding: const EdgeInsets.all(12),
      decoration: BoxDecoration(
        color: ObservatoryTheme.voidBg,
        borderRadius: BorderRadius.circular(8),
        border: Border.all(color: ObservatoryTheme.borderCyan),
      ),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Row(
            children: [
              const Icon(Icons.science, size: 14, color: ObservatoryTheme.cyanCoherence),
              const SizedBox(width: 6),
              Expanded(
                child: Text(
                  p['title'] ?? 'Experiment Proposal',
                  style: const TextStyle(
                    fontSize: 12,
                    fontWeight: FontWeight.bold,
                    fontFamily: 'monospace',
                    color: ObservatoryTheme.cyanCoherence,
                  ),
                ),
              ),
            ],
          ),
          const SizedBox(height: 8),
          Text(
            'Target: ${p['problem']} (${p['hamiltonian']})',
            style: const TextStyle(fontSize: 12, fontWeight: FontWeight.w600),
          ),
          const SizedBox(height: 4),
          Text(
            'Classical FCI Anchor: ${(p['classical_fci_energy'] as num?)?.toStringAsFixed(4)} Ha',
            style: TextStyle(fontSize: 11, color: Colors.white.withOpacity(0.8)),
          ),
          Text(
            'Classical RHF Baseline: ${(p['classical_rhf_energy'] as num?)?.toStringAsFixed(4)} Ha',
            style: TextStyle(fontSize: 11, color: Colors.white.withOpacity(0.8)),
          ),
          Text(
            'Recommended Tier: ${p['recommended_tier']}',
            style: const TextStyle(fontSize: 11, color: ObservatoryTheme.unverifiedAmber),
          ),
          Text(
            'Estimated Cost: \$${(p['estimated_cost_usd'] as num?)?.toStringAsFixed(2)} USD',
            style: const TextStyle(fontSize: 11, color: ObservatoryTheme.corroboratedGreen),
          ),
          const SizedBox(height: 10),
          ElevatedButton.icon(
            style: ElevatedButton.styleFrom(
              backgroundColor: ObservatoryTheme.cyanCoherence.withOpacity(0.18),
              side: const BorderSide(color: ObservatoryTheme.borderCyan),
              padding: const EdgeInsets.symmetric(horizontal: 10, vertical: 6),
            ),
            icon: const Icon(Icons.play_arrow, size: 14, color: ObservatoryTheme.cyanCoherence),
            label: const Text(
              'AUTHORIZE & LAUNCH ON TIER',
              style: TextStyle(fontSize: 10, fontWeight: FontWeight.bold, color: Colors.white),
            ),
            onPressed: () {
              widget.onExperimentTriggered();
              Navigator.of(context).pop();
            },
          ),
        ],
      ),
    );
  }
}
