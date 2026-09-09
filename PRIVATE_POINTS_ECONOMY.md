# Private Points Economy

## Purpose

Private Points (PP) are a persistent individual resource. They represent both everyday purchasing power and strategic liquidity, so routine spending must affect later decisions, negotiations, exams, and expulsion risk.

The engine is authoritative over balances and transactions. Narrative generation may motivate or describe spending, but it must not create, remove, or rewrite PP outside the economic rules.

## Balance

For each student:

```text
ending_balance = opening_balance
               + monthly_income
               + transfers_in
               - transfers_out
               - ordinary_spending
               - event_spending
               - durable_spending
               - strategic_spending
```

A PP balance cannot become negative.

## Monthly income

At each monthly payment, every student receives:

```text
monthly_income = current_class_points * 100
```

The amount uses the student's class state at the payment calculation time and is added to the existing balance. Previous balances are never normalized or reset.

## Spending categories

### Ordinary spending

Recurring optional purchases such as paid food, drinks, snacks, personal products, and entertainment.

The school provides a minimum free level of essential goods/services where applicable. Therefore, a student may remain at `0 PP`; insufficient funds do not create debt or an artificial mandatory expense.

### Event spending

Costs produced by concrete activities such as karaoke, cinema, cafés, restaurants, dates, or other paid social events. The event defines who pays and how the cost is divided.

### Durable spending

Occasional purchases such as clothing, shoes, bags, electronics, school materials, or personal items. These are event/need driven rather than fixed monthly charges.

### Strategic spending

Rule-permitted payments whose primary purpose is competitive or institutional: exam costs, protection from penalties or expulsion, contracts, information, negotiations, or other school mechanisms.

### Transfers

PP may move directly between students when the rules permit it. A transfer is zero-sum: it changes ownership but is not consumption.

## Economic behavior

Students do not share a fixed spending rate. Their economic behavior is derived from character state and stable tendencies such as:

- preference for paid consumption;
- social spending;
- willingness to buy durable/luxury goods;
- saving and strategic reserve behavior;
- willingness to transfer or spend points for other people.

Available balance, current income, anticipated needs, relationships, active goals, and known future risks may modify a student's willingness to spend. A character may deliberately preserve part of the balance as a strategic reserve.

The engine resolves the monetary result from world state and deterministic simulation rules. Character reasoning can influence intent, but cannot directly edit the ledger.

## Ledger and invariants

Every PP movement must create a transaction associated with its cause, participants, amount, category, and simulation time.

The following rules are invariant:

1. PP persist across days, months, events, and exams.
2. Routine living does not imply a fixed mandatory PP deduction.
3. Insufficient funds block the paid transaction or force an available free/cheaper alternative; they never create a negative balance.
4. Social and material actions with a real price must affect the corresponding balances.
5. Transfers conserve PP between participants and are tracked separately from consumption.
6. Strategic payments use the same real balance accumulated through previous simulation history.
7. Balances are never normalized before an exam or narrative event.
8. Later decisions must observe the actual current balance, including the consequences of earlier spending and saving.
