# NetForge Manual Lab Use Cases (No AI Required)

This checklist maps each scenario from `hi.txt` to what you can do **manually** in NetForge today using the canvas, full IOS CLI (`EXEC_CLI`), and WebSocket IPC.

Legend: ✅ supported | ⚠️ partial | ❌ needs external AI / not in scope

---

## 1. Academic & Training

| Use case (hi.txt) | Manual equivalent | Status | How |
|-------------------|-------------------|--------|-----|
| Instant lecture lab generation | Build topology on canvas + configure via CLI | ✅ | Add routers/switches/PCs, cable tool, `configure terminal` |
| Anti-cheating exam variations | Duplicate topology + change IPs/VLANs manually | ✅ | Save/load topology JSON, vary addressing |
| AI homework grader | Activity goals engine | ✅ | `ADD_ACTIVITY_GOAL` + `EVALUATE_ACTIVITY` |
| 24/7 CCNA cram companion | Full IOS CLI troubleshooting | ✅ | `ping`, `traceroute`, `show ip route`, `show vlan`, `show standby` |
| Textbook-to-lab importer | Recreate diagram by hand | ✅ | Canvas + CLI (no auto-import) |

---

## 2. Enterprise & SMB

| Use case | Manual equivalent | Status | How |
|----------|-------------------|--------|-----|
| Call center blueprint | Voice VLAN + QoS + HSRP + LTE backup | ✅ | `switchport voice vlan`, `policy-map`, `standby`, `cellular nr` |
| Multi-site retail branches | OSPF/RIP multi-router template | ✅ | `router ospf`, `network … area`, static routes |
| Legacy config modernizer | Paste configs line-by-line | ⚠️ | `EXEC_CLI` batch / `write memory` |
| Vendor translator (Cisco→Juniper) | — | ❌ | AI-only feature |
| Pre-production staging | Simulate failover in lab | ✅ | HSRP + `shutdown` interface, ping/traceroute |

### Company network tutorial (typical YouTube lab)

| Step | Status | Commands / IPC |
|------|--------|----------------|
| Core + distribution + access switches | ✅ | `ADD_SWITCH`, `vlan`, `vtp domain` |
| VLANs (Sales/Eng/Guest/Voice) | ✅ | `vlan 10`, `switchport access vlan` |
| Trunk between switches | ✅ | `switchport mode trunk`, `switchport trunk allowed vlan` |
| Router-on-a-stick / inter-VLAN | ✅ | `interface Gi0/0.10`, `encapsulation dot1Q` |
| DHCP per VLAN | ✅ | `ip dhcp pool`, `default-router`, `ip dhcp excluded-address` |
| OSPF between sites | ✅ | `router ospf`, `network` |
| NAT/PAT to Internet | ✅ | `ip nat inside/outside`, `ip nat outside source overload` |
| Extended ACL | ✅ | `access-list extended`, `ip access-group` |
| HSRP default gateway | ✅ | `standby 1 ip`, `show standby` |
| VoIP phones + CUCM | ✅ | `ADD_VOIP_PHONE`, `SCCP_REGISTER`, `SIP_CALL` |
| LTE/5G WAN backup | ✅ | `ADD_CELLULAR_GATEWAY`, `ATTACH_5G_NR` |

---

## 3. Cyber Security & Compliance

| Use case | Manual equivalent | Status | How |
|----------|-------------------|--------|-----|
| Penetration test simulation | Trace ACL paths + ping blocked hosts | ✅ | ACL deny rules, event log |
| PCI / compliance audit | Verify VLAN isolation + ACLs | ⚠️ | Manual review + assessment goals |
| Zero-trust architecture | Micro-segmentation with ACLs/VLANs | ✅ | VLANs + extended ACLs per zone |
| Disaster recovery sandbox | Core/ISP failure simulation | ✅ | HSRP failover, link shutdown, `show ip route` |

---

## 4. Cloud & Hybrid

| Use case | Manual equivalent | Status | How |
|----------|-------------------|--------|-----|
| On-prem to AWS VPN | Site-to-site IPsec VPN lab | ✅ | `LOAD_VPN_LAB`, `crypto map`, `NEGOTIATE_IKE`, `show crypto isakmp sa` |
| Export to Terraform/Ansible | — | ❌ | Future IaC export |
| SD-WAN policy simulator | Policy-based routing lab | ⚠️ | Static/OSPF routing; no full SD-WAN |

---

## 5. MSPs & Freelancers

| Use case | Manual equivalent | Status | How |
|----------|-------------------|--------|-----|
| Client pitch diagram | Visual topology on canvas | ✅ | Canvas export (visual) |
| Technical documentation | Running-config + show commands | ✅ | `show running-config`, `show cdp neighbors` |
| As-built inventory | Load configs + list devices | ✅ | `LIST_DEVICES`, `EXPORT_TOPOLOGY` |

---

## Assessment goal types (manual grading)

| Goal type | Checks |
|-----------|--------|
| `ping` | Route exists from host to destination |
| `route_exists` | Router has matching route |
| `ospf_neighbor` | OSPF neighbor present |
| `acl_configured` | Named ACL exists |
| `device_exists` | Device in topology |
| `vlan_configured` | VLAN on switch |
| `hsrp_active` | HSRP group in Active state |
| `nat_configured` | NAT overload or static NAT |
| `dhcp_assigned` | Host has IP address |
| `trunk_configured` | Switch port in trunk mode |

---

## What still requires AI (by design)

- Natural-language lab generation
- Automatic config translation between vendors
- Intelligent “why isn’t DHCP working?” diagnosis
- PDF/textbook topology import
- Terraform/Ansible code generation

Everything else in a standard CCNA company-network lab can be done manually with the engine + UI.
