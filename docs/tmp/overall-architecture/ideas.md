# Ideas

## Basic

* Expose a var or func in REPL that lists available models
* Instruct agent about this var/func in case it wants to call a model


## Saftey

### Agentic Firewall

1. Agents exposed to sensitive data (bank accounts, internal chat, internal codebases)

question
  |              ^
==|== Firewall ==|== --> Security review of prompt
  V              |
research        monitor
websites        inbound
and return      forms
summary

2. Agents exposed to the open internet
    * Exposure to send data (anonymous website)
    * Exposure to receive prompt injections (email)

Example usecase: Assistant who can book a reservation at a restaurant (open internet) and then update personal calendar (private data).

### Agent-built APIs

Dev Agent: builds API that the reservations Agent can use to update personal calendar, exposes this API to reservations Agent.

"On the fly access control via specific-purpose-built APIs"