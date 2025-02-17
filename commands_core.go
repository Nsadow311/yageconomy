package yageconomy

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/jonas747/dcmd"
	"github.com/jonas747/discordgo"
	"github.com/jonas747/dstate"
	"github.com/Nsadow311/yageconomy/models"
	"github.com/Nsadow311/stranger/bot"
	"github.com/Nsadow311/stranger/bot/paginatedmessages"
	"github.com/Nsadow311/stranger/commands"
	"github.com/Nsadow311/stranger/common"
	"github.com/pkg/errors"
	"github.com/volatiletech/sqlboiler/boil"
	"github.com/volatiletech/sqlboiler/queries/qm"
	"strconv"
	"strings"
	"time"
)

var CoreCommands = []*commands.YAGCommand{
	&commands.YAGCommand{
		CmdCategory: CategoryEconomy,
		Name:        "$",
		Aliases:     []string{"balance", "wallet"},
		Description: "Shows you balance",
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.AdvUserNoMember},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			var targetAccount *models.EconomyUser
			var target *discordgo.User
			conf := CtxConfig(parsed.Context())

			if parsed.Args[0].Value != nil {
				target = parsed.Args[0].User()

				var err error
				targetAccount, _, err = GetCreateAccount(parsed.Context(), target.ID, parsed.GS.ID, conf.StartBalance)
				if err != nil {
					return nil, err
				}
			} else {
				target = parsed.Msg.Author
				targetAccount = CtxUser(parsed.Context())
			}

			embed := &discordgo.MessageEmbed{
				Author:      UserEmebdAuthor(target),
				Description: "Account of " + target.Username,
				Color:       ColorBlue,
				Fields: []*discordgo.MessageEmbedField{
					&discordgo.MessageEmbedField{
						Inline: true,
						Name:   "Bank Balance",
						Value:  conf.CurrencySymbol + fmt.Sprint(targetAccount.MoneyBank),
					},
					&discordgo.MessageEmbedField{
						Inline: true,
						Name:   "Wallet",
						Value:  conf.CurrencySymbol + fmt.Sprint(targetAccount.MoneyWallet),
					},
					&discordgo.MessageEmbedField{
						Inline: true,
						Name:   "Gambling profit boost %",
						Value:  fmt.Sprintf("%d%%", targetAccount.GamblingBoostPercentage),
					},
					&discordgo.MessageEmbedField{
						Inline: true,
						Name:   "Fish caught",
						Value:  fmt.Sprintf("%d", targetAccount.FishCaugth),
					},
				},
			}
			return embed, nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Withdraw",
		Description:  "Withdraws money from your bank account into your wallet",
		RequiredArgs: 1,
		Middlewares:  []dcmd.MiddleWareFunc{moneyAlteringMW},
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Amount", Type: &AmountArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			account := CtxUser(parsed.Context())
			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			amount, resp := parsed.Args[0].Value.(*AmountArgResult).ApplyWithRestrictions(account.MoneyBank, conf.CurrencySymbol, "bank account", true, 1)
			if resp != "" {
				return ErrorEmbed(u, resp), nil
			}

			account.MoneyBank -= int64(amount)
			account.MoneyWallet += int64(amount)
			_, err := account.UpdateG(parsed.Context(), boil.Whitelist("money_bank", "money_wallet"))
			if err != nil {
				return nil, err
			}

			return SimpleEmbedResponse(u, "Withdrew **%s%d** from your bank, your wallet now has **%s%d**", conf.CurrencySymbol, amount, conf.CurrencySymbol, account.MoneyWallet), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Deposit",
		Description:  "Deposits money into your bank account from your wallet",
		Middlewares:  []dcmd.MiddleWareFunc{moneyAlteringMW},
		RequiredArgs: 1,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Amount", Type: &AmountArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			account := CtxUser(parsed.Context())
			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			amount, resp := parsed.Args[0].Value.(*AmountArgResult).ApplyWithRestrictions(account.MoneyWallet, conf.CurrencySymbol, "wallet", true, 1)
			if resp != "" {
				return ErrorEmbed(u, resp), nil
			}

			account.MoneyBank += int64(amount)
			account.MoneyWallet -= int64(amount)
			_, err := account.UpdateG(parsed.Context(), boil.Whitelist("money_bank", "money_wallet"))
			if err != nil {
				return nil, err
			}

			return SimpleEmbedResponse(u, "Deposited **%s%d** Into your bank account, your bank now contains **%s%d**", conf.CurrencySymbol, amount, conf.CurrencySymbol, account.MoneyBank), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Give",
		Description:  "Give someone money from your wallet",
		Middlewares:  []dcmd.MiddleWareFunc{moneyAlteringMW},
		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.AdvUserNoMember},
			&dcmd.ArgDef{Name: "Amount", Type: &AmountArg{}},
			&dcmd.ArgDef{Name: "Reason", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			target := parsed.Args[0].User()

			account := CtxUser(parsed.Context())
			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			amount, resp := parsed.Args[1].Value.(*AmountArgResult).ApplyWithRestrictions(account.MoneyWallet, conf.CurrencySymbol, "wallet", true, 1)
			if resp != "" {
				return ErrorEmbed(u, resp), nil
			}

			targetAccount, _, err := GetCreateAccount(parsed.Context(), target.ID, parsed.GS.ID, conf.StartBalance)
			if err != nil {
				return nil, err
			}

			targetAccount.MoneyWallet += int64(amount)
			account.MoneyWallet -= int64(amount)

			// update the acconts
			err = TransferMoneyWallet(parsed.Context(), nil, conf, false, u.ID, target.ID, int64(amount), int64(amount))
			if err != nil {
				return nil, err
			}

			extraStr := ""
			if parsed.Args[2].Str() != "" {
				extraStr = " with the message: **" + parsed.Args[2].Str() + "**"
			}

			return SimpleEmbedResponse(u, "Sent %s%d to %s%s", conf.CurrencySymbol, amount, target.Username, extraStr), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory: CategoryEconomy,
		Name:        "Daily",
		Middlewares: []dcmd.MiddleWareFunc{moneyAlteringMW},
		Description: "Claim your daily free cash",
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			account := CtxUser(parsed.Context())
			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			if conf.DailyAmount < 1 {
				return ErrorEmbed(u, "Daily not set up on this server"), nil
			}

			result, err := common.PQ.Exec(`UPDATE economy_users SET last_daily_claim = now(), money_wallet = money_wallet + $4
			WHERE guild_id = $1 AND user_id = $2 AND EXTRACT(EPOCH FROM (now() - last_daily_claim))  > $3`, parsed.GS.ID, u.ID, conf.DailyFrequency*60, conf.DailyAmount)
			if err != nil {
				return nil, err
			}
			rows, err := result.RowsAffected()
			if err != nil {
				return nil, err
			}

			if rows > 0 {
				return SimpleEmbedResponse(u, "Claimed your daily of **%s%d**", conf.CurrencySymbol, conf.DailyAmount), nil
			}

			timeToWait := account.LastDailyClaim.Add(time.Duration(conf.DailyFrequency) * time.Minute).Sub(time.Now())
			return ErrorEmbed(u, "You can't claim your daily yet again! Please wait another %s.", common.HumanizeDuration(common.DurationPrecisionSeconds, timeToWait)), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory: CategoryEconomy,
		Name:        "TopMoney",
		Aliases:     []string{"LB"},
		Description: "Economy leaderboard, optionally specify a page",
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Page", Type: dcmd.Int, Default: 1},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			conf := CtxConfig(parsed.Context())

			page := parsed.Args[0].Int()
			if page < 0 {
				page = 1
			}

			_, err := paginatedmessages.CreatePaginatedMessage(parsed.GS.ID, parsed.CS.ID, page, 0, func(p *paginatedmessages.PaginatedMessage, newPage int) (*discordgo.MessageEmbed, error) {

				offset := (newPage - 1) * 10
				if offset < 0 {
					offset = 0
				}

				result, err := models.EconomyUsers(
					models.EconomyUserWhere.GuildID.EQ(parsed.GS.ID),
					qm.OrderBy("money_wallet + money_bank desc"),
					qm.Limit(10),
					qm.Offset(offset)).AllG(context.Background())
				if err != nil {
					return nil, err
				}

				embed := SimpleEmbedResponse(parsed.Msg.Author, "")
				embed.Title = conf.CurrencySymbol + " Leaderboard"

				userIDs := make([]int64, len(result))
				for i, v := range result {
					userIDs[i] = v.UserID
				}

				members, err := bot.GetMembers(parsed.GS.ID, userIDs...)
				// users := bot.GetUsersGS(parsed.GS, userIDs...)

				for i, v := range result {
					user := ""
					for _, m := range members {
						if m.ID == v.UserID {
							user = m.Nick
							if user == "" {
								user = m.Username
							}
							break
						}
					}

					if user == "" {
						user = fmt.Sprintf("user left (ID: %d)", v.UserID)
					}

					embed.Fields = append(embed.Fields, &discordgo.MessageEmbedField{
						Name:  fmt.Sprintf("#%d %s", i+offset+1, user),
						Value: fmt.Sprintf("%s%d", conf.CurrencySymbol, v.MoneyBank+v.MoneyWallet),
					})
				}

				return embed, nil
				// return SimpleEmbedResponse(ms, buf.String()), nil
			})

			return nil, err
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Plant",
		Description:  "Plants a certain amount of currency in the channel, optionally with a password, use Pick to pick it",
		Middlewares:  []dcmd.MiddleWareFunc{moneyAlteringMW},
		RequiredArgs: 1,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Money", Type: &AmountArg{}},
			&dcmd.ArgDef{Name: "Password", Type: dcmd.String, Default: ""},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			account := CtxUser(parsed.Context())
			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			amount, resp := parsed.Args[0].Value.(*AmountArgResult).ApplyWithRestrictions(account.MoneyWallet, conf.CurrencySymbol, "wallet", true, 10)
			if resp != "" {
				return ErrorEmbed(u, resp), nil
			}

			_, err := models.FindEconomyPlantG(parsed.Context(), parsed.CS.ID)
			if err == nil {
				return ErrorEmbed(u, "There's already money planted in this channel"), nil
			}

			cmdPrefix, _ := commands.GetCommandPrefix(conf.GuildID)
			msgContent := fmt.Sprintf("%s planted **%s%d** in the channel!\nUse `%spick (code-here)` to pick it up", u.Username, conf.CurrencySymbol, amount, cmdPrefix)

			err = PlantMoney(parsed.Context(), conf, parsed.CS.ID, u.ID, int(amount), parsed.Args[1].Str(), msgContent)
			if err != nil {
				return nil, err
			}

			err = TransferMoneyWallet(parsed.Context(), nil, conf, false, u.ID, common.BotUser.ID, amount, amount)
			if err != nil {
				return nil, err
			}

			bot.MessageDeleteQueue.DeleteMessages(parsed.CS.ID, parsed.Msg.ID)

			return nil, nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Pick",
		Description:  "Picks up money planted in the channel previously using plant",
		RequiredArgs: 1,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Password", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			bot.MessageDeleteQueue.DeleteMessages(parsed.CS.ID, parsed.Msg.ID)

			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			p, err := models.EconomyPlants(
				models.EconomyPlantWhere.ChannelID.EQ(parsed.CS.ID),
				models.EconomyPlantWhere.Password.EQ(strings.ToLower(parsed.Args[0].Str())),
				qm.OrderBy("message_id desc"),
			).OneG(parsed.Context())

			if err != nil {
				if errors.Cause(err) == sql.ErrNoRows {
					return ErrorEmbed(u, "No plant in this channel or incorrect passowrd :("), nil
				}

				return nil, err
			}

			noPlant := false
			pmAmount := int64(0)
			err = common.SqlTX(func(tx *sql.Tx) error {
				pm, err := models.EconomyPlants(models.EconomyPlantWhere.MessageID.EQ(p.MessageID), qm.For("update")).One(parsed.Context(), tx)
				if err != nil {
					if errors.Cause(err) == sql.ErrNoRows {
						noPlant = true
					}
					return err
				}
				if pm.MessageID != p.MessageID {
					noPlant = true
					return nil
				}

				pmAmount = pm.Amount

				_, err = tx.Exec("UPDATE economy_users SET money_wallet = money_wallet + $3 WHERE user_id = $2 AND guild_id = $1", parsed.GS.ID, u.ID, pm.Amount)
				if err != nil {
					return err
				}

				_, err = pm.Delete(parsed.Context(), tx)
				return err
			})

			if noPlant {
				return ErrorEmbed(u, "Yikes, someone snatched it before you."), nil
			}

			if err != nil {
				return nil, err
			}

			common.BotSession.ChannelMessageDelete(parsed.CS.ID, p.MessageID)

			return SimpleEmbedResponse(u, fmt.Sprintf("Picked up **%s%d**!", conf.CurrencySymbol, pmAmount)), nil
		},
	},
}

var CoreAdminCommands = []*commands.YAGCommand{
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Award",
		Description:  "Award a member of the server some money (admins only)",
		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.AdvUserNoMember},
			&dcmd.ArgDef{Name: "Amount", Type: &dcmd.IntArg{Min: 1, Max: 0xfffffffffffffff}},
			&dcmd.ArgDef{Name: "Reason", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			target := parsed.Args[0].User()

			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			amount := parsed.Args[1].Int()

			// esnure that the account exists
			_, _, err := GetCreateAccount(parsed.Context(), target.ID, parsed.GS.ID, conf.StartBalance)
			if err != nil {
				return nil, err
			}

			_, err = common.PQ.Exec("UPDATE economy_users SET money_bank = money_bank + $3 WHERE guild_id = $1 AND user_id = $2", parsed.GS.ID, target.ID, amount)
			if err != nil {
				return nil, err
			}

			extraStr := ""
			if parsed.Args[2].Str() != "" {
				extraStr = " with the message: **" + parsed.Args[2].Str() + "**"
			}

			return SimpleEmbedResponse(u, "Awarded **%s** with %s%d%s", target.Username, conf.CurrencySymbol, amount, extraStr), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "AwardAll",
		Description:  "Award all members with the target role",
		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.String},
			&dcmd.ArgDef{Name: "Amount", Type: &dcmd.IntArg{Min: 1, Max: 0xfffffffffffffff}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {

			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			target := FindRole(parsed.GS, parsed.Args[0].Str())
			if target == nil {
				return ErrorEmbed(u, "Unknown role"), nil
			}

			amount := parsed.Args[1].Int64()

			bot.BatchMemberJobManager.NewBatchMemberJob(parsed.GS.ID, func(g int64, members []*discordgo.Member) {
				numAwarded := 0
				for _, m := range members {
					if !common.ContainsInt64Slice(m.Roles, target.ID) {
						continue
					}
					numAwarded++

					// esnure that the account exists
					_, created, err := GetCreateAccount(context.Background(), m.User.ID, g, conf.StartBalance+amount)
					if err != nil {
						logger.WithError(err).Error("failed retrieving account")
						return
					}

					if !created {
						_, err = common.PQ.Exec("UPDATE economy_users SET money_bank = money_bank + $3 WHERE guild_id = $1 AND user_id = $2", parsed.GS.ID, m.User.ID, amount)
						if err != nil {
							logger.WithError(err).Error("failed awarding money")
						}
					}
				}

				common.BotSession.ChannelMessageSendEmbed(parsed.CS.ID, SimpleEmbedResponse(u, "Gave %d members **%s%d**", numAwarded, conf.CurrencySymbol, amount))
			})

			return SimpleEmbedResponse(u, "Started the job..."), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "Take",
		Description:  "Takes away money from someone (admins only)",
		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.AdvUserNoMember},
			&dcmd.ArgDef{Name: "Amount", Type: &AmountArg{}},
			&dcmd.ArgDef{Name: "Reason", Type: dcmd.String},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			target := parsed.Args[0].User()

			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			// esnure that the account exists
			tAccount, _, err := GetCreateAccount(parsed.Context(), target.ID, parsed.GS.ID, conf.StartBalance)
			if err != nil {
				return nil, err
			}

			amount, resp := parsed.Args[1].Value.(*AmountArgResult).ApplyWithRestrictions(tAccount.MoneyWallet+tAccount.MoneyBank, conf.CurrencySymbol, "wallet", false, 1)
			if resp != "" {
				return ErrorEmbed(u, resp), nil
			}

			_, err = common.PQ.Exec("UPDATE economy_users SET money_wallet = money_wallet - $3 WHERE guild_id = $1 AND user_id = $2", parsed.GS.ID, target.ID, amount)
			if err != nil {
				return nil, err
			}

			extraStr := ""
			if parsed.Args[2].Str() != "" {
				extraStr = " with the message: **" + parsed.Args[2].Str() + "**"
			}

			return SimpleEmbedResponse(u, "Took away %s%d from **%s**%s", conf.CurrencySymbol, amount, target.Username, extraStr), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "TakeAll",
		Description:  "Takes away money from all the users with the role",
		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.String},
			&dcmd.ArgDef{Name: "Amount", Type: &AmountArg{}},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {

			conf := CtxConfig(parsed.Context())
			u := parsed.Msg.Author

			target := FindRole(parsed.GS, parsed.Args[0].Str())
			if target == nil {
				return ErrorEmbed(u, "Unknown role"), nil
			}

			bot.BatchMemberJobManager.NewBatchMemberJob(parsed.GS.ID, func(g int64, members []*discordgo.Member) {
				numTaken := 0
				for _, m := range members {
					if !common.ContainsInt64Slice(m.Roles, target.ID) {
						continue
					}
					numTaken++

					// esnure that the account exists
					account, _, err := GetCreateAccount(context.Background(), m.User.ID, g, conf.StartBalance)
					if err != nil {
						logger.WithError(err).Error("failed retrieving account")
						return
					}

					amount := parsed.Args[1].Value.(*AmountArgResult).Apply(account.MoneyBank + account.MoneyWallet)
					account.MoneyWallet -= amount
					if account.MoneyWallet < 0 {
						account.MoneyBank += account.MoneyWallet
					}

					_, err = account.UpdateG(context.Background(), boil.Whitelist("money_wallet", "money_bank"))
					if err != nil {
						logger.WithError(err).Error("failed taking away money")
					}
				}

				common.BotSession.ChannelMessageSendEmbed(parsed.CS.ID, SimpleEmbedResponse(u, "Took away from %d members", numTaken))
			})

			return SimpleEmbedResponse(u, "Started the job..."), nil
		},
	},
	&commands.YAGCommand{
		CmdCategory:  CategoryEconomy,
		Name:         "DelUser",
		Description:  "Deletes someone's economy account (admins only)",
		RequiredArgs: 2,
		Arguments: []*dcmd.ArgDef{
			&dcmd.ArgDef{Name: "Target", Type: dcmd.AdvUserNoMember},
		},
		RunFunc: func(parsed *dcmd.Data) (interface{}, error) {
			target := parsed.Args[0].User()

			u := parsed.Msg.Author

			n, err := models.EconomyUsers(models.EconomyUserWhere.GuildID.EQ(parsed.GS.ID), models.EconomyUserWhere.UserID.EQ(target.ID)).DeleteAll(parsed.Context(), common.PQ)
			if err != nil {
				return nil, err
			}

			if n < 1 {
				return ErrorEmbed(u, "That user did not have an account"), nil
			}

			return SimpleEmbedResponse(u, "Deleted economy account owned by **%s#%s**", target.Username, target.Discriminator), nil
		},
	},
}

func FindRole(gs *dstate.GuildState, searchStr string) *discordgo.Role {
	gs.RLock()
	defer gs.RUnlock()

	parsedSearch, _ := strconv.ParseInt(searchStr, 10, 64)

	if strings.HasPrefix(searchStr, "<@&") && strings.HasSuffix(searchStr, ">") {
		// attempt to parse the mention
		trimmed := strings.TrimPrefix(searchStr, "<@&")
		trimmed = strings.TrimSuffix(searchStr, ">")
		parsedSearch, _ = strconv.ParseInt(trimmed, 10, 64)
	}

	// incase it was just @ and the role name
	searchTrimedPrefix := strings.TrimPrefix(searchStr, "@")

	for _, v := range gs.Guild.Roles {
		if parsedSearch != 0 && v.ID == parsedSearch {
			return v
		}

		if strings.EqualFold(searchStr, v.Name) {
			return v
		}

		if strings.EqualFold(searchTrimedPrefix, v.Name) {
			return v
		}

	}

	return nil
}
