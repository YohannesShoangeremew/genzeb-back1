# 🎯 Bingo Game Rules - Step by Step Guide

Welcome to Bingo! This guide will walk you through everything you need to know to play and win.

## 📋 Table of Contents

1. [Game Overview](#game-overview)
2. [Getting Started](#getting-started)
3. [How to Play](#how-to-play)
4. [Game States](#game-states)
5. [How to Win](#how-to-win)
6. [Prize Calculation](#prize-calculation)
7. [Important Rules](#important-rules)
8. [Tips & Strategies](#tips--strategies)

---

## 🎮 Game Overview

### What is Bingo?
Bingo is a game of chance where players mark numbers on a 5×5 card as they are randomly drawn. The first player to complete a winning pattern (row, column, diagonal, or four corners) wins the prize pool!

### Game Types & Bet Amounts

There are two game tiers:

| Game Type   | Bet Amount | Prize Pool (with 10 players) |
|-------------|------------|------------------------------|
| **REGULAR** | 10         | 80 (80% of 100)              |
| **VIP**     | 50         | 400 (80% of 500)             |

**Note:** 20% of each bet goes to the house, 80% goes to the prize pool.

---

## 🚀 Getting Started

### Step 1: Check Your Balance
- Make sure you have enough balance in your wallet to cover the bet amount
- You need at least the bet amount (10 for REGULAR, 50 for VIP)

### Step 2: Choose a Game Type
- Select a game type: REGULAR (10) or VIP (50)
- The VIP tier has a higher bet and a bigger prize pool!

### Step 3: Join a Game
- Find an available game of your chosen type
- If no game exists, one will be created automatically
- You can join games in **WAITING** or **COUNTDOWN** states

### Step 4: Select Your Card
- Choose a card ID from 1 to 100
- **Multiple players can select the same card** - it's allowed!
- Your bet is **immediately deducted** when you join

---

## 🎲 How to Play

### The Bingo Card

Your card is a 5×5 grid with 5 columns labeled **B-I-N-G-O**:

```
     B    I    N    G    O
  ┌────┬────┬────┬────┬────┐
  │ 15 │ 28 │ 31 │ 58 │ 61 │  Row 1
  ├────┼────┼────┼────┼────┤
  │  5 │ 30 │ 45 │ 46 │ 73 │  Row 2
  ├────┼────┼────┼────┼────┤
  │  3 │ 16 │  0 │ 48 │ 65 │  Row 3 (center is FREE)
  ├────┼────┼────┼────┼────┤
  │  1 │ 20 │ 33 │ 50 │ 75 │  Row 4
  ├────┼────┼────┼────┼────┤
  │ 12 │ 18 │ 43 │ 60 │ 63 │  Row 5
  └────┴────┴────┴────┴────┘
```

**Number Ranges:**
- **B column**: Numbers 1-15
- **I column**: Numbers 16-30
- **N column**: Numbers 31-45 (center cell is FREE - always marked)
- **G column**: Numbers 46-60
- **O column**: Numbers 61-75

**Important:** The center cell (row 3, column 3) is always FREE and counts as marked automatically!

---

## 📊 Game States

The game progresses through these states:

### 1. **WAITING** ⏳
- Game is open for players to join
- Requires minimum **2 players** to start
- You can join or leave (full refund if you leave)

### 2. **COUNTDOWN** ⏰
- Starts automatically when the 2nd player joins
- **60-second countdown** before the game begins
- You can still join during countdown
- You can leave during countdown (full refund)
- If players drop below 2, countdown stops and game returns to WAITING

### 3. **DRAWING** 🎲
- Numbers are drawn **every 1 second**
- Watch for drawn numbers in real-time
- Mark matching numbers on your card
- **You can claim BINGO at any time**
- **No refunds** if you leave during drawing

### 4. **FINISHED** ✅
- Winner has been declared
- Prize has been distributed
- A new game automatically starts

### 5. **CANCELLED** ❌
- All players were eliminated (invalid bingo claims)
- All players receive refunds
- A new game automatically starts

---

## 🏆 How to Win

### Winning Patterns

You win by completing **any one** of these patterns:

1. **Any Row** (5 numbers horizontally)
   ```
   [X] [X] [X] [X] [X]
   ```

2. **Any Column** (5 numbers vertically)
   ```
   [X]
   [X]
   [X]
   [X]
   [X]
   ```

3. **Any Diagonal** (5 numbers diagonally)
   ```
   [X]                    [X]
      [X]              [X]
         [X]        [X]
            [X]  [X]
               [X]
   ```

4. **Four Corners** (4 corner numbers)
   ```
   [X]                 [X]


   [X]                 [X]
   ```

### How to Claim Bingo

1. **Mark your numbers** as they are drawn
2. When you complete a winning pattern, **claim BINGO immediately**
3. Send your claim with the **positions** (0-24) of your marked numbers
4. The server validates your claim:
   - ✅ **Valid Bingo**: You WIN! Prize is added to your wallet
   - ❌ **Invalid Bingo**: You are ELIMINATED (no refund)

### Position Numbering

Positions are numbered **left-to-right, top-to-bottom** (0-24):

```
Position:  0   1   2   3   4
          ┌───┬───┬───┬───┬───┐
          │ 0 │ 1 │ 2 │ 3 │ 4 │  Row 1
          ├───┼───┼───┼───┼───┤
          │ 5 │ 6 │ 7 │ 8 │ 9 │  Row 2
          ├───┼───┼───┼───┼───┤
          │10 │11 │12 │13 │14 │  Row 3 (12 is center - FREE)
          ├───┼───┼───┼───┼───┤
          │15 │16 │17 │18 │19 │  Row 4
          ├───┼───┼───┼───┼───┤
          │20 │21 │22 │23 │24 │  Row 5
          └───┴───┴───┴───┴───┘
```

**Example:** If you mark the top row, send positions: `[0, 1, 2, 3, 4]`

---

## 💰 Prize Calculation

### How Prizes Work

- **Prize Pool** = (Bet Amount × Number of Players) × 80%
- **House Cut** = 20% (goes to the house)
- **Winner receives** = Entire prize pool

### Example Calculations

**VIP Game (Bet: 50) with 10 players:**
- Total bets: 50 × 10 = 500
- House cut (20%): 100
- Prize pool (80%): 400
- **Winner receives: 400**

**REGULAR Game (Bet: 10) with 5 players:**
- Total bets: 10 × 5 = 50
- House cut (20%): 10
- Prize pool (80%): 40
- **Winner receives: 40**

**More players = Bigger prizes!** 🎉

---

## ⚠️ Important Rules

### ✅ DO's

- ✅ Join games with sufficient balance
- ✅ Mark numbers as they are drawn
- ✅ Claim BINGO as soon as you complete a pattern
- ✅ You can leave during WAITING or COUNTDOWN (full refund)
- ✅ Multiple players can select the same card ID
- ✅ Watch games in real-time via WebSocket

### ❌ DON'Ts

- ❌ Don't claim BINGO if you haven't completed a pattern (you'll be eliminated)
- ❌ Don't claim with numbers that weren't drawn
- ❌ Don't leave during DRAWING phase (no refund)
- ❌ Don't join games that are already FINISHED or CANCELLED
- ❌ Don't claim BINGO if you're already eliminated

### ⚡ Key Points

1. **Bet is deducted immediately** when you join
2. **Refunds only** during WAITING or COUNTDOWN states
3. **No refunds** during DRAWING phase
4. **Invalid bingo claim = elimination** (no refund)
5. **First valid claim wins** - be fast!
6. **Center cell is always FREE** (position 12)
7. **New games start automatically** after each game ends

---

## 💡 Tips & Strategies

### Strategy Tips

1. **Watch Multiple Games**
   - You can watch games via WebSocket without joining
   - Learn the patterns before betting

2. **Start Small**
   - Begin with the REGULAR (10 bet) tier to learn the game
   - Move up to VIP (50 bet) once comfortable

3. **Be Quick**
   - First valid claim wins
   - Have your positions ready when claiming

4. **Check Your Card**
   - Review your card before joining
   - Know which numbers you need

5. **Monitor Drawn Numbers**
   - Keep track of all drawn numbers
   - Watch for patterns forming

6. **Join Early**
   - More players = bigger prize pool
   - Join during WAITING for best chance

### Common Mistakes to Avoid

- ❌ Claiming too early (before completing pattern)
- ❌ Claiming with wrong positions
- ❌ Not including the center cell in diagonal claims
- ❌ Claiming with numbers that weren't drawn
- ❌ Joining games you can't afford

---

## 🎯 Quick Reference

### Game Flow Summary

```
1. Choose game type (REGULAR or VIP)
2. Join game → Bet deducted
3. Select card (1-100)
4. Wait for 2nd player → Countdown starts (60s)
5. Game begins → Numbers drawn every 1s
6. Mark numbers on your card
7. Complete pattern → Claim BINGO
8. Win → Prize added to wallet! 🎉
```

### Winning Patterns Quick Check

- ✅ Any row (5 horizontal)
- ✅ Any column (5 vertical)
- ✅ Any diagonal (5 diagonal)
- ✅ Four corners
- ✅ Center cell is FREE (always marked)

### Position Reference

```
Top row:     [0, 1, 2, 3, 4]
Middle row:  [10, 11, 12, 13, 14] (12 is center - FREE)
Bottom row:  [20, 21, 22, 23, 24]
Left column: [0, 5, 10, 15, 20]
Right column:[4, 9, 14, 19, 24]
```

---

## 📞 Need Help?

If you have questions or encounter issues:

1. Check your wallet balance
2. Verify you're in the correct game state
3. Ensure your card positions are correct (0-24)
4. Make sure all claimed numbers were actually drawn

**Remember:** The server validates all claims - be accurate!

---

## 🎉 Good Luck!

Now you're ready to play! Join a game, select your card, and may the best player win! 🍀

**Happy Bingo!** 🎯
